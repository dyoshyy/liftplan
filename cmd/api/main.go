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
	"syscall"
	"time"

	"github.com/dyoshyy/liftplan-server/internal/application/usecase"
	"github.com/dyoshyy/liftplan-server/internal/domain/training"
	"github.com/dyoshyy/liftplan-server/internal/domain/training/seed"
	"github.com/dyoshyy/liftplan-server/internal/infrastructure/memory"
	"github.com/dyoshyy/liftplan-server/internal/presentation/httpapi"
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
	handler, err := buildHandler()
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

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

// buildHandler は依存を組み立てる。テストからも呼べるよう main と分けている。
func buildHandler() (http.Handler, error) {
	pool, err := seed.Exercises()
	if err != nil {
		return nil, fmt.Errorf("種目シードが不正: %w", err)
	}
	program, err := defaultProgram(pool)
	if err != nil {
		return nil, err
	}

	exercises := memory.NewExerciseRepository(pool)
	logs := memory.NewSetLogRepository()
	conditions := memory.NewConditionRepository()
	programs := memory.NewProgramRepository(program)

	planner := training.DefaultSessionPlanner()

	handler := httpapi.NewHandler(
		usecase.NewGetSession(exercises, logs, conditions, programs, planner),
		usecase.NewRecordSets(logs, exercises),
		usecase.NewRecordConditions(conditions),
		usecase.NewConfigureProgram(exercises, programs),
		usecase.NewGetProgram(programs),
	)
	return handler.Routes(), nil
}

// defaultProgram はシードから初期プログラムを組む。
//
// バリエーションはメインに付随して自動で回るため、選択には含めない。
// 初期値を入れておくのは、起動直後に PUT /api/program を叩かないと
// 何も使えない状態を避けるため。設定はいつでも上書きできる。
func defaultProgram(pool []*training.Exercise) (*training.Program, error) {
	freq, err := training.NewFrequency(defaultFrequencyPerWeek)
	if err != nil {
		return nil, fmt.Errorf("既定の頻度が不正: %w", err)
	}

	target, err := seed.DefaultWeeklyTarget(freq)
	if err != nil {
		return nil, fmt.Errorf("週目標シードが不正: %w", err)
	}

	selected := make([]training.ExerciseID, 0, len(pool))
	for _, e := range pool {
		if e.Kind() != training.KindVariation {
			selected = append(selected, e.ID())
		}
	}

	return training.NewProgram(freq, target, selected)
}
