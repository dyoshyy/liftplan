// Command api は liftplan のサーバーを起動する。
//
// Onion Architecture において、具象を知ってよいのはこの層だけ。
// リポジトリ実装を Postgres に差し替えるときも、変更はこのファイルに閉じるはずで、
// それが依存方向を守れているかの試金石になる。
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/dyoshyy/liftplan-server/internal/application/query"
	"github.com/dyoshyy/liftplan-server/internal/application/usecase"
	"github.com/dyoshyy/liftplan-server/internal/domain/training/seed"
	"github.com/dyoshyy/liftplan-server/internal/infrastructure/memory"
	"github.com/dyoshyy/liftplan-server/internal/infrastructure/postgres"
	"github.com/dyoshyy/liftplan-server/internal/presentation/httpapi"

	"github.com/dyoshyy/liftplan-server/internal/domain/training/condition"
	"github.com/dyoshyy/liftplan-server/internal/domain/training/exercise"
	"github.com/dyoshyy/liftplan-server/internal/domain/training/planning"
	"github.com/dyoshyy/liftplan-server/internal/domain/training/program"
	"github.com/dyoshyy/liftplan-server/internal/domain/training/setlog"
)

const defaultFrequencyPerWeek = 3

// タイムアウトはスローロリス対策。既定の http.Server は無制限で、
// ヘッダを1バイトずつ送るだけで接続を占有できる。
const (
	readHeaderTimeout = 5 * time.Second
	readTimeout       = 15 * time.Second
	writeTimeout      = 30 * time.Second
	idleTimeout       = 60 * time.Second

	// requestTimeout はハンドラに渡す context の期限。
	//
	// これが無いと、リクエストの context に期限が付かず 504 を返す経路が
	// 本番で発火しない。WriteTimeout より短くするのは、応答を書く余地を
	// 残すため。同じにすると 504 を書く前に接続が切られる。
	requestTimeout = 20 * time.Second

	// shutdownTimeout は最長のリクエスト予算より長く取る。
	// 短いと、サーバー自身が許可した長さのリクエストを必ず切ることになり、
	// クライアントは保存できたのか分からないまま再送する。
	shutdownTimeout = writeTimeout + 5*time.Second

	// healthCheckTimeout はヘルスチェックが保存先の応答を待つ上限。
	// 長いと、詰まった DB のせいでヘルスチェック自体が詰まる。
	healthCheckTimeout = 2 * time.Second
)

func main() {
	if err := run(); err != nil {
		// 何に失敗したのかはエラー自身が持つ。ここで一律に
		// 「起動に失敗」と書くと、停止の失敗も listen の失敗も
		// シードの不正も同じ文言になり、ログから原因が読めない。
		slog.Error("サーバーが異常終了した", "error", err)
		os.Exit(1)
	}
}

func run() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	handler, closeRepos, err := buildHandler(ctx)
	if err != nil {
		return err
	}
	defer closeRepos()

	return serve(ctx, stop, newServer(handler, ":"+port()))
}

// newServer はタイムアウトを設定した HTTP サーバーを組む。
func newServer(handler http.Handler, addr string) *http.Server {
	return &http.Server{
		Addr: addr,
		// リクエストの context に期限を付ける。付けないと、
		// ハンドラは切断以外の理由で打ち切られることがない。
		Handler:           withTimeout(handler, requestTimeout),
		ReadHeaderTimeout: readHeaderTimeout,
		ReadTimeout:       readTimeout,
		WriteTimeout:      writeTimeout,
		IdleTimeout:       idleTimeout,
	}
}

// serve はサーバーを起動し、ctx が終わったら停止する。
//
// SIGTERM で即座に接続を切らない。処理中のリクエストを落とすと、
// クライアントは保存できたのか分からないまま再送する。
//
// stop はシグナルの購読解除。main と分けているのはテストのため。
func serve(ctx context.Context, stop func(), srv *http.Server) error {
	ln, err := net.Listen("tcp", srv.Addr)
	if err != nil {
		return fmt.Errorf("listen に失敗: %w", err)
	}
	return serveListener(ctx, stop, srv, ln)
}

// serveListener は listener を受け取る版。テストが任意のポートを使えるようにする。
func serveListener(ctx context.Context, stop func(), srv *http.Server, ln net.Listener) error {
	errs := make(chan error, 1)
	go func() {
		slog.Info("liftplan-server を起動する", "addr", srv.Addr)
		if err := srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errs <- err
		}
		close(errs)
	}()

	select {
	case err := <-errs:
		return err
	case <-ctx.Done():
		slog.Info("停止シグナルを受け取った。処理中のリクエストを待つ",
			"timeout", shutdownTimeout)
	}

	// シグナルの購読を解除して既定の動作に戻す。解除しないと、
	// 待っている間に押した2回目の Ctrl-C が握り潰され、
	// 強制停止の逃げ道が kill -9 しか無くなる。
	stop()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		// 待ちきれなかった場合でも接続を閉じてから返す。
		// 閉じないとプロセスが listen したまま終了する。
		_ = srv.Close()
		return fmt.Errorf("停止に失敗: %w", err)
	}
	slog.Info("停止した")
	return nil
}

// withTimeout はリクエストの context に期限を付ける。
//
// http.TimeoutHandler を使わないのは、あちらが独自の 503 を書いてしまい、
// プレゼンテーション層のステータス分類（D-042）を迂回するため。
// context に期限を付けるだけにして、応答の形はハンドラに任せる。
func withTimeout(next http.Handler, d time.Duration) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), d)
		defer cancel()
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func port() string {
	if v := os.Getenv("PORT"); v != "" {
		return v
	}
	return "8080"
}

// 配線層は読みと書きの両方を持つ。ドメインが口を半分ずつに割っている
// のは「使う側が要る分だけ受け取る」ためで、実装を組み立てる側まで
// 半分にすると、同じインスタンスを2つのフィールドに入れることになる。
type setLogStore interface {
	setlog.Reader
	setlog.Writer
}

type conditionStore interface {
	condition.Reader
	condition.Writer
}

type programStore interface {
	program.Reader
	program.Writer
}

// repositories は差し替えの対象になる口の集まり。
//
// この構造体があるのは、インメモリと Postgres の選択を1箇所に閉じるため。
// 組み立ての途中に条件分岐が散ると、どちらの実装が使われているかが
// 読めなくなる。
type repositories struct {
	exercises  exercise.Reader
	logs       setLogStore
	conditions conditionStore
	programs   programStore
	// ping は保存先に到達できるかを確かめる。インメモリなら常に成功する。
	ping  func(context.Context) error
	close func()
}

// buildHandler は依存を組み立てる。テストからも呼べるよう main と分けている。
func buildHandler(ctx context.Context) (http.Handler, func(), error) {
	pool, err := seed.Exercises()
	if err != nil {
		return nil, nil, fmt.Errorf("種目シードが不正: %w", err)
	}

	repos, err := openRepositories(ctx, pool)
	if err != nil {
		return nil, nil, err
	}

	exercises := repos.exercises
	logs := repos.logs
	conditions := repos.conditions
	programs := repos.programs

	planner := planning.DefaultSessionPlanner()

	handler := httpapi.NewHandler(
		usecase.NewGetSession(exercises, logs, conditions, programs, planner),
		usecase.NewRecordSets(logs, exercises),
		usecase.NewRecordConditions(conditions),
		usecase.NewConfigureProgram(exercises, programs),
		usecase.NewGetProgram(programs),
		usecase.NewDeleteSetLog(logs),
		query.NewExercises(exercises),
		query.NewHistory(logs, exercises),
		query.NewStats(logs, exercises, programs, planning.DefaultOneRepMaxEstimator()),
	)
	guarded, err := withAuth(handler.Routes())
	if err != nil {
		repos.close()
		return nil, nil, err
	}
	shared, err := withCORS(guarded)
	if err != nil {
		repos.close()
		return nil, nil, err
	}
	return withHealthCheck(shared, repos.ping), repos.close, nil
}

// minTokenLength は認証トークンの最短の長さ。
//
// 短いトークンは総当たりで破れる。32文字は 128bit 相当を16進で書いた長さで、
// `openssl rand -hex 16` の出力がちょうどこれになる。
const minTokenLength = 32

// withAuth は認証を要求する。
//
// トークンが未設定なら起動しない。「未設定なら認証しない」にすると、
// 環境変数の設定漏れがそのまま全公開になる。起動しないほうが、
// 気づかないまま公開されるよりずっとよい。
func withAuth(next http.Handler) (http.Handler, error) {
	token := os.Getenv("AUTH_TOKEN")
	switch {
	case token == "":
		return nil, fmt.Errorf(
			"AUTH_TOKEN が設定されていない。`openssl rand -hex 32` などで生成すること")
	case len(token) < minTokenLength:
		return nil, fmt.Errorf(
			"AUTH_TOKEN が短すぎる: %d文字（最低 %d文字）", len(token), minTokenLength)
	}
	return httpapi.RequireBearerToken(token)(next), nil
}

// withCORS は画面のオリジンからのクロスオリジン要求を許す。
//
// 認証の外側に被せる。preflight の OPTIONS にはブラウザが Authorization を
// 付けないので、内側に置くと必ず 401 になる。
//
// AUTH_TOKEN と同じく未設定なら起動しない。既定で全部許すと、設定漏れが
// そのまま「どのサイトからでもトークン付きで叩ける」状態になる。既定で
// 何も許さないほうは、設定漏れが「画面が動かない」として静かに出るだけで、
// 原因に辿り着くまで時間がかかる。起動しないのが一番早く気づく。
func withCORS(next http.Handler) (http.Handler, error) {
	raw := os.Getenv("ALLOWED_ORIGINS")
	if raw == "" {
		return nil, fmt.Errorf(
			"ALLOWED_ORIGINS が設定されていない。画面のオリジンをカンマ区切りで指定すること" +
				"（例: https://liftplan-web.example.workers.dev,http://localhost:5173）")
	}

	origins := strings.Split(raw, ",")
	for i, o := range origins {
		origins[i] = strings.TrimSpace(o)
	}
	return httpapi.AllowOrigins(origins)(next), nil
}

// withHealthCheck はヘルスチェックを保存先の疎通込みで応答する。
//
// プレゼンテーション層は保存先を知らないので、ここで被せる。
// 疎通を見ないヘルスチェックは、何も処理できないインスタンスを
// 「健全」と報告し続け、ロードバランサがトラフィックを流し込む。
func withHealthCheck(next http.Handler, ping func(context.Context) error) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != httpapi.HealthPath {
			next.ServeHTTP(w, r)
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), healthCheckTimeout)
		defer cancel()

		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		if err := ping(ctx); err != nil {
			slog.Error("ヘルスチェックが失敗", "error", err)
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(`{"status":"unavailable"}`))
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})
}

// openRepositories は DATABASE_URL があれば Postgres、無ければインメモリを返す。
//
// インメモリを残すのは、ドメインの検証を DB 無しで回せる状態を捨てないため。
// 「とりあえず動かす」ための逃げ道でもある。
//
// 種目マスタだけは常にインメモリ。シードはバイナリ同梱の静的なマスタで、
// DB に置くとマイグレーションのたびに種目の追加・改名が絡み、
// ErrExerciseNotFound の意味が「まだ流していない」と混ざる。
func openRepositories(ctx context.Context, pool []*exercise.Exercise) (repositories, error) {
	exercises := memory.NewExerciseRepository(pool)

	url := os.Getenv("DATABASE_URL")
	if url == "" {
		slog.Warn("DATABASE_URL が無いのでインメモリで動く。再起動すると記録は消える")
		prog, err := defaultProgram(pool)
		if err != nil {
			return repositories{}, err
		}
		return repositories{
			exercises:  exercises,
			logs:       memory.NewSetLogRepository(),
			conditions: memory.NewConditionRepository(),
			programs:   memory.NewProgramRepository(prog),
			ping:       func(context.Context) error { return nil },
			close:      func() {},
		}, nil
	}

	db, err := postgres.Open(ctx, url)
	if err != nil {
		return repositories{}, err
	}
	// マイグレーションは起動時に流す。手で流す運用にすると、
	// 流し忘れたインスタンスが古いスキーマに書き込む。
	if err := postgres.Migrate(ctx, db); err != nil {
		db.Close()
		return repositories{}, err
	}

	programs := postgres.NewProgramRepository(db)
	if err := seedProgramIfMissing(ctx, programs, pool); err != nil {
		db.Close()
		return repositories{}, err
	}

	slog.Info("Postgres に接続した")
	return repositories{
		exercises:  exercises,
		logs:       postgres.NewSetLogRepository(db),
		conditions: postgres.NewConditionRepository(db),
		programs:   programs,
		ping:       db.Ping,
		close:      db.Close,
	}, nil
}

// seedProgramIfMissing は未設定なら初期プログラムを入れる。
//
// 空のデータベースから始めたユーザーが、PUT /api/program を叩かないと
// 何も使えない状態を避ける。すでに設定があれば触らない。
func seedProgramIfMissing(
	ctx context.Context,
	programs programStore,
	pool []*exercise.Exercise,
) error {
	switch _, err := programs.Get(ctx); {
	case err == nil:
		return nil
	case !errors.Is(err, program.ErrProgramNotConfigured):
		return fmt.Errorf("プログラムの確認に失敗: %w", err)
	}

	prog, err := defaultProgram(pool)
	if err != nil {
		return err
	}
	if err := programs.Save(ctx, prog); err != nil {
		return fmt.Errorf("初期プログラムを保存できない: %w", err)
	}
	slog.Info("初期プログラムを保存した", "per_week", prog.Frequency().PerWeek())
	return nil
}

// defaultProgram はシードから初期プログラムを組む。
//
// バリエーションはメインに付随して自動で回るため、選択には含めない。
// 初期値を入れておくのは、起動直後に PUT /api/program を叩かないと
// 何も使えない状態を避けるため。設定はいつでも上書きできる。
func defaultProgram(pool []*exercise.Exercise) (*program.Program, error) {
	freq, err := program.NewFrequency(defaultFrequencyPerWeek)
	if err != nil {
		return nil, fmt.Errorf("既定の頻度が不正: %w", err)
	}

	target, err := seed.DefaultWeeklyTarget(freq)
	if err != nil {
		return nil, fmt.Errorf("週目標シードが不正: %w", err)
	}

	// 全種目を選んでおく。外したいものはあとから設定で外せる。
	selected := make([]exercise.ExerciseID, 0, len(pool))
	for _, e := range pool {
		selected = append(selected, e.ID())
	}
	return program.NewProgram(freq, target, selected, seed.DefaultDeclared())
}
