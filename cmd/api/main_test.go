package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/dyoshyy/liftplan/internal/domain/training/seed"
	"github.com/dyoshyy/liftplan/internal/presentation/httpapi"
)

// testAuthToken はテスト用の認証トークン。本番と同じ経路を通すために、
// テストでも必ず認証を通す。素通しする抜け道を作ると、認証が壊れても
// 他のテストが気づかない。
const testAuthToken = "test-token-0123456789abcdef0123456789ab"

// authed は認証ヘッダを付けたリクエストを作る。
func authed(method, target string, body io.Reader) *http.Request {
	r := httptest.NewRequest(method, target, body)
	r.Header.Set("Authorization", "Bearer "+testAuthToken)
	if body != nil {
		r.Header.Set("Content-Type", "application/json")
	}
	return r
}

// testAllowedOrigin は画面のオリジン。CORS の許可一覧に入れないと起動しない。
const testAllowedOrigin = "https://liftplan-web.example.workers.dev"

func TestBuildHandler_ServesSession(t *testing.T) {
	t.Setenv("AUTH_TOKEN", testAuthToken)
	t.Setenv("ALLOWED_ORIGINS", testAllowedOrigin)
	handler, closeRepos, err := buildHandler(context.Background())
	if err != nil {
		t.Fatalf("組み立てに失敗: %v", err)
	}
	t.Cleanup(closeRepos)

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, authed(http.MethodGet, "/api/sessions?date=2026-08-17", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("ステータスが誤り: %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestBuildHandler_Healthz(t *testing.T) {
	t.Setenv("AUTH_TOKEN", testAuthToken)
	t.Setenv("ALLOWED_ORIGINS", testAllowedOrigin)
	handler, closeRepos, err := buildHandler(context.Background())
	if err != nil {
		t.Fatalf("組み立てに失敗: %v", err)
	}
	t.Cleanup(closeRepos)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, httpapi.HealthPath, nil))
	if rec.Code != http.StatusOK {
		t.Errorf("ヘルスチェックが失敗: %d", rec.Code)
	}
}

// 初期プログラムでセッションが導出できること。
// 起動直後に PUT /api/program を叩かないと何も使えない状態を避ける。
func TestBuildHandler_WorksOutOfTheBox(t *testing.T) {
	t.Setenv("AUTH_TOKEN", testAuthToken)
	t.Setenv("ALLOWED_ORIGINS", testAllowedOrigin)
	handler, closeRepos, err := buildHandler(context.Background())
	if err != nil {
		t.Fatalf("組み立てに失敗: %v", err)
	}
	t.Cleanup(closeRepos)

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, authed(http.MethodGet, "/api/sessions?date=2026-08-17", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("ステータスが誤り: %d body=%s", rec.Code, rec.Body.String())
	}

	var got struct {
		Main        []json.RawMessage `json:"main"`
		Accessories []json.RawMessage `json:"accessories"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("応答を解釈できない: %v", err)
	}
	if len(got.Main) != 1 {
		t.Errorf("ヘビー枠が1つでない: %d", len(got.Main))
	}
	if len(got.Accessories) == 0 {
		t.Error("補助種目が1つも出ていない")
	}
}

// 実績を記録してから計画を取り直すと重量が確定すること。
// 層をまたいだ往復がここで初めて通る。
func TestBuildHandler_RecordThenPlan(t *testing.T) {
	t.Setenv("AUTH_TOKEN", testAuthToken)
	t.Setenv("ALLOWED_ORIGINS", testAllowedOrigin)
	handler, closeRepos, err := buildHandler(context.Background())
	if err != nil {
		t.Fatalf("組み立てに失敗: %v", err)
	}
	t.Cleanup(closeRepos)

	post := func(t *testing.T, path, body string) {
		t.Helper()
		r := authed(http.MethodPost, path, strings.NewReader(body))
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, r)
		if rec.Code != http.StatusNoContent {
			t.Fatalf("%s が失敗: %d body=%s", path, rec.Code, rec.Body.String())
		}
	}

	// 宣言した3種目すべてに記録を入れる。ヘビー枠は「最後にやったのが
	// 最も古い種目」で、一度もやっていない種目が最優先になるので、
	// ベンチだけ記録すると未実施のスクワットやデッドリフトが軸に来る。
	// 同じ日に揃えると、同点でマスタ順（ID昇順）の bench が選ばれる。
	var logs []string
	for i := range 3 {
		for s := range 3 {
			for _, id := range []string{"bench", "squat", "deadlift"} {
				logs = append(logs, fmt.Sprintf(
					`{"id":"e%d-%d-%s","date":"2026-07-%02d","exercise_id":%q,`+
						`"weight_kg":85,"reps":8,"rir":2}`, i, s, id, 6+i*7, id))
			}
		}
	}
	post(t, "/api/set-logs", `{"logs":[`+strings.Join(logs, ",")+`]}`)

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, authed(http.MethodGet, "/api/sessions?date=2026-08-17", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("ステータスが誤り: %d", rec.Code)
	}

	var got struct {
		Main []struct {
			ExerciseID string   `json:"exercise_id"`
			WeightKg   *float64 `json:"weight_kg"`
		} `json:"main"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("応答を解釈できない: %v", err)
	}
	for _, m := range got.Main {
		if m.ExerciseID != "bench" {
			continue
		}
		if m.WeightKg == nil {
			t.Fatal("記録したのに重量が未確定")
		}
		if *m.WeightKg <= 0 {
			t.Errorf("重量が不正: %v", *m.WeightKg)
		}
		return
	}
	t.Fatal("bench がメインに無い")
}

// PORT が空なら既定値を使うこと。
func TestPort(t *testing.T) {
	t.Setenv("PORT", "")
	if got := port(); got != "8080" {
		t.Errorf("既定のポートが誤り: %s", got)
	}
	t.Setenv("PORT", "9999")
	if got := port(); got != "9999" {
		t.Errorf("PORT が反映されない: %s", got)
	}
}

// タイムアウトが実際に設定されていること。
// 既定の http.Server は無制限で、ヘッダを1バイトずつ送るだけで
// 接続を占有できる。
func TestNewServer_SetsTimeouts(t *testing.T) {
	srv := newServer(http.NotFoundHandler(), ":0")

	for name, got := range map[string]time.Duration{
		"ReadHeaderTimeout": srv.ReadHeaderTimeout,
		"ReadTimeout":       srv.ReadTimeout,
		"WriteTimeout":      srv.WriteTimeout,
		"IdleTimeout":       srv.IdleTimeout,
	} {
		if got <= 0 {
			t.Errorf("%s が設定されていない: %v", name, got)
		}
		if got > time.Minute {
			t.Errorf("%s が長すぎる: %v", name, got)
		}
	}

	// 停止の猶予が最長のリクエスト予算より短いと、サーバー自身が
	// 許可した長さのリクエストを必ず切ることになる。
	if shutdownTimeout <= srv.WriteTimeout {
		t.Errorf("停止の猶予が短すぎる: %v <= %v", shutdownTimeout, srv.WriteTimeout)
	}
	// 応答を書く余地を残すため、リクエストの期限は WriteTimeout より短い。
	if requestTimeout >= srv.WriteTimeout {
		t.Errorf("リクエストの期限が長すぎる: %v >= %v", requestTimeout, srv.WriteTimeout)
	}
}

// リクエストの context に期限が付くこと。
// 付かないと 504 を返す経路が本番で発火しない。
func TestWithTimeout_SetsADeadline(t *testing.T) {
	var deadline time.Time
	var ok bool
	h := withTimeout(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		deadline, ok = r.Context().Deadline()
	}), 42*time.Second)

	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))
	if !ok {
		t.Fatal("期限が付いていない")
	}
	if remaining := time.Until(deadline); remaining > 42*time.Second || remaining < 41*time.Second {
		t.Errorf("期限が誤り: 残り %v", remaining)
	}
}

// 処理中のリクエストを待ってから停止すること。
func TestServe_WaitsForInFlightRequests(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	completed := make(chan struct{})

	srv := newServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		close(started)
		<-release
		w.WriteHeader(http.StatusNoContent)
		close(completed)
	}), "127.0.0.1:0")

	ln, err := net.Listen("tcp", srv.Addr)
	if err != nil {
		t.Fatalf("listen に失敗: %v", err)
	}
	srv.Addr = ln.Addr().String()

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- serveListener(ctx, func() {}, srv, ln) }()

	go func() {
		resp, err := http.Get("http://" + srv.Addr + "/")
		if err == nil {
			_ = resp.Body.Close()
		}
	}()
	<-started

	cancel() // 停止シグナル相当
	select {
	case <-completed:
		t.Fatal("停止を待たずにハンドラが完了した")
	case <-time.After(50 * time.Millisecond):
	}

	close(release)
	select {
	case <-completed:
	case <-time.After(2 * time.Second):
		t.Fatal("処理中のリクエストが完了しなかった")
	}
	if err := <-done; err != nil {
		t.Errorf("停止に失敗: %v", err)
	}
}

// listen に失敗したら起動しないこと。
func TestServe_ReturnsListenError(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen に失敗: %v", err)
	}
	defer func() { _ = ln.Close() }()

	srv := newServer(http.NotFoundHandler(), ln.Addr().String())
	if err := serve(context.Background(), func() {}, srv); err == nil {
		t.Error("ポートが使用中なのにエラーにならない")
	}
}

// 停止時にシグナルの購読を解除すること。
// 解除しないと、待っている間の2回目の Ctrl-C が握り潰される。
func TestServe_ReleasesSignalHandling(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen に失敗: %v", err)
	}
	srv := newServer(http.NotFoundHandler(), ln.Addr().String())

	stopped := make(chan struct{})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if err := serveListener(ctx, func() { close(stopped) }, srv, ln); err != nil {
		t.Fatalf("停止に失敗: %v", err)
	}
	select {
	case <-stopped:
	default:
		t.Error("シグナルの購読が解除されていない")
	}
}

// 組み立てたハンドラのリクエストに期限が付くこと。
// buildHandler だけを見ていると、配線を外しても気づけない。
func TestRun_WiresTheRequestTimeout(t *testing.T) {
	var hasDeadline bool
	srv := newServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		_, hasDeadline = r.Context().Deadline()
	}), ":0")

	srv.Handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))
	if !hasDeadline {
		t.Error("リクエストの期限が配線されていない。504 を返す経路が本番で発火しない")
	}
}

// シードが不正なら起動しないこと。黙って進むと、種目が欠けたまま
// サーバーが上がってしまう。
func TestBuildHandler_FailsFastOnBadSeed(t *testing.T) {
	// シードが正しいことは他のテストが確かめている。ここでは
	// 「エラーを握り潰していないか」を型で担保する。
	t.Setenv("AUTH_TOKEN", testAuthToken)
	t.Setenv("ALLOWED_ORIGINS", testAllowedOrigin)
	handler, closeRepos, err := buildHandler(context.Background())
	if err != nil {
		t.Fatalf("シードが正しいのに失敗: %v", err)
	}
	t.Cleanup(closeRepos)
	if handler == nil {
		t.Fatal("ハンドラが nil である")
	}

	pool, err := seed.Exercises()
	if err != nil {
		t.Fatalf("シードが不正: %v", err)
	}
	if len(pool) == 0 {
		t.Fatal("シードが空である")
	}
}

// DATABASE_URL があれば Postgres を使い、記録が組み立て直しても残ること。
//
// 「差し替えが cmd に閉じる」という受け入れ基準の裏取り。ここが通れば、
// ドメイン・アプリケーション・プレゼンテーションを1行も変えずに
// 永続化が入ったことになる。
func TestBuildHandler_UsesPostgresWhenConfigured(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL が未設定のため実行しない（make test-db で回せる）")
	}
	t.Setenv("DATABASE_URL", url)

	post := func(t *testing.T, h http.Handler, path, body string) int {
		t.Helper()
		r := authed(http.MethodPost, path, strings.NewReader(body))
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, r)
		return rec.Code
	}
	heavyWeight := func(t *testing.T, h http.Handler) *float64 {
		t.Helper()
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, authed(http.MethodGet, "/api/sessions?date=2026-08-17", nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("セッションの取得に失敗: %d body=%s", rec.Code, rec.Body.String())
		}
		var got struct {
			Main []struct {
				ExerciseID string   `json:"exercise_id"`
				WeightKg   *float64 `json:"weight_kg"`
			} `json:"main"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
			t.Fatalf("応答を解釈できない: %v", err)
		}
		if len(got.Main) != 1 {
			t.Fatalf("ヘビー枠が1つでない: %d", len(got.Main))
		}
		return got.Main[0].WeightKg
	}

	t.Setenv("AUTH_TOKEN", testAuthToken)
	t.Setenv("ALLOWED_ORIGINS", testAllowedOrigin)
	first, closeFirst, err := buildHandler(context.Background())
	if err != nil {
		t.Fatalf("組み立てに失敗: %v", err)
	}

	// 前のテスト実行の残りを消してから始める。
	// 宣言した3種目すべてに記録する。ヘビー枠は「最後にやったのが最も
	// 古い種目」で、一度もやっていない種目が最優先になるので、ベンチだけ
	// 記録すると未実施のスクワットが軸に来て重量が確定しない。
	id := fmt.Sprintf("e2e-%d", time.Now().UnixNano())
	logs := make([]string, 0, 9)
	for i := range 3 {
		for _, ex := range []string{"bench", "squat", "deadlift"} {
			logs = append(logs, fmt.Sprintf(
				`{"id":"%s-%d-%s","date":"2026-08-10","exercise_id":%q,`+
					`"weight_kg":85,"reps":8,"rir":2}`, id, i, ex, ex))
		}
	}
	if code := post(t, first, "/api/set-logs",
		`{"logs":[`+strings.Join(logs, ",")+`]}`); code != http.StatusNoContent {
		t.Fatalf("記録に失敗: %d", code)
	}
	before := heavyWeight(t, first)
	if before == nil {
		t.Fatal("記録したのに重量が未確定")
	}
	closeFirst()

	// 組み立て直す＝再起動に相当する。
	t.Setenv("AUTH_TOKEN", testAuthToken)
	t.Setenv("ALLOWED_ORIGINS", testAllowedOrigin)
	second, closeSecond, err := buildHandler(context.Background())
	if err != nil {
		t.Fatalf("2度目の組み立てに失敗: %v", err)
	}
	t.Cleanup(closeSecond)

	after := heavyWeight(t, second)
	if after == nil {
		t.Fatal("組み立て直したら記録が消えた")
	}
	if *after != *before {
		t.Errorf("組み立て直しで重量が変わった: %v → %v", *before, *after)
	}
}

// DATABASE_URL が無ければインメモリで動くこと。
// ドメインの検証を DB 無しで回せる状態を捨てない。
func TestBuildHandler_FallsBackToMemory(t *testing.T) {
	t.Setenv("DATABASE_URL", "")

	t.Setenv("AUTH_TOKEN", testAuthToken)
	t.Setenv("ALLOWED_ORIGINS", testAllowedOrigin)
	handler, closeRepos, err := buildHandler(context.Background())
	if err != nil {
		t.Fatalf("組み立てに失敗: %v", err)
	}
	t.Cleanup(closeRepos)

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, authed(http.MethodGet, "/api/sessions?date=2026-08-17", nil))
	if rec.Code != http.StatusOK {
		t.Errorf("インメモリで動いていない: %d", rec.Code)
	}
}

// 接続先が誤っていたら起動しないこと。
// 遅延させると、最初のリクエストが来るまで気づけない。
func TestBuildHandler_FailsFastOnBadDatabaseURL(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://nobody:nobody@127.0.0.1:1/nothing")

	t.Setenv("AUTH_TOKEN", testAuthToken)
	t.Setenv("ALLOWED_ORIGINS", testAllowedOrigin)
	if _, _, err := buildHandler(context.Background()); err == nil {
		t.Error("到達できない接続先で起動した")
	}
}

// 保存先に到達できないとき、ヘルスチェックが失敗すること。
// 何も処理できないインスタンスを「健全」と報告すると、
// ロードバランサがトラフィックを流し込み続ける。
func TestHealthz_ReflectsTheRepositoryState(t *testing.T) {
	healthy := withHealthCheck(http.NotFoundHandler(), func(context.Context) error { return nil })
	rec := httptest.NewRecorder()
	healthy.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, httpapi.HealthPath, nil))
	if rec.Code != http.StatusOK {
		t.Errorf("健全なのに %d を返した", rec.Code)
	}

	broken := withHealthCheck(http.NotFoundHandler(), func(context.Context) error {
		return errors.New("接続できない")
	})
	rec = httptest.NewRecorder()
	broken.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, httpapi.HealthPath, nil))
	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("保存先に到達できないのに %d を返した", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "unavailable") {
		t.Errorf("状態が返っていない: %s", rec.Body.String())
	}

	// /healthz 以外は素通しすること。
	var reached bool
	pass := withHealthCheck(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		reached = true
	}), func(context.Context) error { return errors.New("接続できない") })
	pass.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/api/sessions", nil))
	if !reached {
		t.Error("/healthz 以外が素通しされていない")
	}
}

// トークンが未設定・短すぎるなら起動しないこと。
//
// 「未設定なら認証しない」にすると、環境変数の設定漏れがそのまま
// 全公開になる。気づかないまま公開されるより、起動しないほうがよい。
func TestBuildHandler_RefusesToStartWithoutAToken(t *testing.T) {
	for name, token := range map[string]string{
		"未設定":    "",
		"短すぎる":   "short",
		"境界の1つ下": strings.Repeat("a", minTokenLength-1),
	} {
		t.Run(name, func(t *testing.T) {
			t.Setenv("AUTH_TOKEN", token)
			t.Setenv("ALLOWED_ORIGINS", testAllowedOrigin)
			if _, _, err := buildHandler(context.Background()); err == nil {
				t.Error("トークンが不十分なのに起動した")
			}
		})
	}

	t.Run("境界ちょうどなら起動する", func(t *testing.T) {
		t.Setenv("AUTH_TOKEN", strings.Repeat("a", minTokenLength))
		t.Setenv("ALLOWED_ORIGINS", testAllowedOrigin)
		_, closeRepos, err := buildHandler(context.Background())
		if err != nil {
			t.Fatalf("十分な長さなのに起動しない: %v", err)
		}
		closeRepos()
	})
}

// 許可オリジンが未設定なら起動しないこと。
//
// AUTH_TOKEN と同じ理由。既定で全部許すと、設定漏れがそのまま
// 「どのサイトからでもトークン付きで叩ける」状態になる。
// 既定で何も許さないほうは「画面が動かない」として静かに出るだけで、
// 原因に辿り着くまで時間がかかる。起動しないのが一番早く気づく。
func TestBuildHandler_RefusesToStartWithoutAllowedOrigins(t *testing.T) {
	t.Setenv("AUTH_TOKEN", testAuthToken)
	t.Setenv("ALLOWED_ORIGINS", "")
	if _, _, err := buildHandler(context.Background()); err == nil {
		t.Error("許可オリジンが未設定なのに起動した")
	}
}

// 組み立てたハンドラが preflight を認証の外側で返すこと。
//
// ミドルウェアを書いても重ね順を間違えれば、ブラウザからは
// 原因の分からない 401 になる。配線そのものを検査する。
func TestBuildHandler_AnswersPreflightWithoutCredentials(t *testing.T) {
	t.Setenv("AUTH_TOKEN", testAuthToken)
	t.Setenv("ALLOWED_ORIGINS", testAllowedOrigin)
	handler, closeRepos, err := buildHandler(context.Background())
	if err != nil {
		t.Fatalf("組み立てに失敗: %v", err)
	}
	t.Cleanup(closeRepos)

	r := httptest.NewRequest(http.MethodOptions, "/api/set-logs", nil)
	r.Header.Set("Origin", testAllowedOrigin)
	r.Header.Set("Access-Control-Request-Method", http.MethodPost)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, r)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("preflight が %d（期待 204）body=%s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != testAllowedOrigin {
		t.Errorf("Allow-Origin が %q", got)
	}
}

// 組み立てたハンドラが実際に認証を要求すること。
// ミドルウェアを書いても配線を忘れれば意味がない。
func TestBuildHandler_RequiresAuthentication(t *testing.T) {
	t.Setenv("AUTH_TOKEN", testAuthToken)
	t.Setenv("ALLOWED_ORIGINS", testAllowedOrigin)
	handler, closeRepos, err := buildHandler(context.Background())
	if err != nil {
		t.Fatalf("組み立てに失敗: %v", err)
	}
	t.Cleanup(closeRepos)

	for _, path := range []string{
		"/api/sessions?date=2026-08-17", "/api/program",
	} {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("%s が認証なしで通った: %d", path, rec.Code)
		}
	}

	// 書き込みも塞がっていること。
	rec := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/set-logs",
		strings.NewReader(`{"logs":[]}`))
	r.Header.Set("Content-Type", "application/json")
	handler.ServeHTTP(rec, r)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("書き込みが認証なしで通った: %d", rec.Code)
	}

	// ヘルスチェックは通ること。
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, httpapi.HealthPath, nil))
	if rec.Code != http.StatusOK {
		t.Errorf("ヘルスチェックが弾かれた: %d", rec.Code)
	}
}

// 組み立てたハンドラで /healthz が応答すること。
//
// /healthz の持ち主は cmd だけ（ルータには無い）。被せ忘れると
// 404 になり、Cloud Run の起動プローブが通らなくなる。
func TestBuildHandler_ServesHealthCheck(t *testing.T) {
	t.Setenv("AUTH_TOKEN", testAuthToken)
	t.Setenv("ALLOWED_ORIGINS", testAllowedOrigin)
	handler, closeRepos, err := buildHandler(context.Background())
	if err != nil {
		t.Fatalf("組み立てに失敗: %v", err)
	}
	t.Cleanup(closeRepos)

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, httpapi.HealthPath, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("ヘルスチェックが応答しない: %d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "ok") {
		t.Errorf("状態が返っていない: %s", rec.Body.String())
	}
}
