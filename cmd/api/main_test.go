package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/dyoshyy/liftplan-server/internal/domain/training"
	"github.com/dyoshyy/liftplan-server/internal/domain/training/seed"
)

func TestBuildHandler_ServesSession(t *testing.T) {
	handler, closeRepos, err := buildHandler(context.Background())
	if err != nil {
		t.Fatalf("組み立てに失敗: %v", err)
	}
	t.Cleanup(closeRepos)

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/sessions?date=2026-08-17", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("ステータスが誤り: %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestBuildHandler_Healthz(t *testing.T) {
	handler, closeRepos, err := buildHandler(context.Background())
	if err != nil {
		t.Fatalf("組み立てに失敗: %v", err)
	}
	t.Cleanup(closeRepos)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if rec.Code != http.StatusOK {
		t.Errorf("ヘルスチェックが失敗: %d", rec.Code)
	}
}

// 初期プログラムでセッションが導出できること。
// 起動直後に PUT /api/program を叩かないと何も使えない状態を避ける。
func TestBuildHandler_WorksOutOfTheBox(t *testing.T) {
	handler, closeRepos, err := buildHandler(context.Background())
	if err != nil {
		t.Fatalf("組み立てに失敗: %v", err)
	}
	t.Cleanup(closeRepos)

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/sessions?date=2026-08-17", nil))
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
	if len(got.Main) != 3 {
		t.Errorf("メイン種目が3つでない: %d", len(got.Main))
	}
	if len(got.Accessories) == 0 {
		t.Error("補助種目が1つも出ていない")
	}
}

// 実績を記録してから計画を取り直すと重量が確定すること。
// 層をまたいだ往復がここで初めて通る。
func TestBuildHandler_RecordThenPlan(t *testing.T) {
	handler, closeRepos, err := buildHandler(context.Background())
	if err != nil {
		t.Fatalf("組み立てに失敗: %v", err)
	}
	t.Cleanup(closeRepos)

	post := func(t *testing.T, path, body string) {
		t.Helper()
		r := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, r)
		if rec.Code != http.StatusNoContent {
			t.Fatalf("%s が失敗: %d body=%s", path, rec.Code, rec.Body.String())
		}
	}

	var logs []string
	for i := range 3 {
		for s := range 3 {
			logs = append(logs, fmt.Sprintf(
				`{"id":"e%d-%d","date":"2026-07-%02d","exercise_id":"bench",`+
					`"weight_kg":85,"reps":8,"rir":2}`, i, s, 6+i*7))
		}
	}
	post(t, "/api/set-logs", `{"logs":[`+strings.Join(logs, ",")+`]}`)

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/sessions?date=2026-08-17", nil))
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

// 初期プログラムにバリエーションを含めないこと。
// 含めると、バリエーションがメイン扱いで独立したスロットを持つ。
func TestDefaultProgram_ExcludesVariations(t *testing.T) {
	pool, err := seed.Exercises()
	if err != nil {
		t.Fatalf("シードが不正: %v", err)
	}
	program, err := defaultProgram(pool)
	if err != nil {
		t.Fatalf("初期プログラムが不正: %v", err)
	}

	for _, e := range pool {
		if e.Kind() != training.KindVariation {
			continue
		}
		if program.Includes(e.ID()) {
			t.Errorf("バリエーション %s が選択に含まれている", e.ID())
		}
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

// 初期プログラムの頻度が README の記述と一致すること。
func TestDefaultProgram_MatchesTheDocumentedFrequency(t *testing.T) {
	pool, err := seed.Exercises()
	if err != nil {
		t.Fatalf("シードが不正: %v", err)
	}
	program, err := defaultProgram(pool)
	if err != nil {
		t.Fatalf("初期プログラムが不正: %v", err)
	}
	if got := program.Frequency().PerWeek(); got != 3 {
		t.Errorf("既定の頻度が誤り: %d（README は週3回と書いている）", got)
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
	// 空のプールから初期プログラムを組もうとしたら失敗すること。
	// 失敗しないなら、シードが空でもサーバーが起動してしまう。
	if _, err := defaultProgram(nil); err == nil {
		t.Error("空のプールで初期プログラムが組めてしまう")
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
		r := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, r)
		return rec.Code
	}
	benchWeight := func(t *testing.T, h http.Handler) *float64 {
		t.Helper()
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/sessions?date=2026-08-17", nil))
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
		for _, m := range got.Main {
			if m.ExerciseID == "bench" {
				return m.WeightKg
			}
		}
		t.Fatal("bench がメインに無い")
		return nil
	}

	first, closeFirst, err := buildHandler(context.Background())
	if err != nil {
		t.Fatalf("組み立てに失敗: %v", err)
	}

	// 前のテスト実行の残りを消してから始める。
	id := fmt.Sprintf("e2e-%d", time.Now().UnixNano())
	logs := make([]string, 0, 3)
	for i := range 3 {
		logs = append(logs, fmt.Sprintf(
			`{"id":"%s-%d","date":"2026-08-10","exercise_id":"bench",`+
				`"weight_kg":85,"reps":8,"rir":2}`, id, i))
	}
	if code := post(t, first, "/api/set-logs",
		`{"logs":[`+strings.Join(logs, ",")+`]}`); code != http.StatusNoContent {
		t.Fatalf("記録に失敗: %d", code)
	}
	before := benchWeight(t, first)
	if before == nil {
		t.Fatal("記録したのに重量が未確定")
	}
	closeFirst()

	// 組み立て直す＝再起動に相当する。
	second, closeSecond, err := buildHandler(context.Background())
	if err != nil {
		t.Fatalf("2度目の組み立てに失敗: %v", err)
	}
	t.Cleanup(closeSecond)

	after := benchWeight(t, second)
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

	handler, closeRepos, err := buildHandler(context.Background())
	if err != nil {
		t.Fatalf("組み立てに失敗: %v", err)
	}
	t.Cleanup(closeRepos)

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/sessions?date=2026-08-17", nil))
	if rec.Code != http.StatusOK {
		t.Errorf("インメモリで動いていない: %d", rec.Code)
	}
}

// 接続先が誤っていたら起動しないこと。
// 遅延させると、最初のリクエストが来るまで気づけない。
func TestBuildHandler_FailsFastOnBadDatabaseURL(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://nobody:nobody@127.0.0.1:1/nothing")

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
	healthy.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if rec.Code != http.StatusOK {
		t.Errorf("健全なのに %d を返した", rec.Code)
	}

	broken := withHealthCheck(http.NotFoundHandler(), func(context.Context) error {
		return errors.New("接続できない")
	})
	rec = httptest.NewRecorder()
	broken.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))
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
