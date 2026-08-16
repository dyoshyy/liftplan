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
	shutdownTimeout   = 10 * time.Second
)

func main() {
	if err := run(); err != nil {
		slog.Error("起動に失敗", "error", err)
		os.Exit(1)
	}
}

func run() error {
	handler, err := buildHandler()
	if err != nil {
		return err
	}

	srv := &http.Server{
		Addr:              ":" + port(),
		Handler:           handler,
		ReadHeaderTimeout: readHeaderTimeout,
		ReadTimeout:       readTimeout,
		WriteTimeout:      writeTimeout,
		IdleTimeout:       idleTimeout,
	}

	// SIGTERM で即座に接続を切らない。処理中のリクエストを
	// 落とすと、クライアントは保存できたのか分からないまま再送する。
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	errs := make(chan error, 1)
	go func() {
		slog.Info("liftplan-server を起動する", "addr", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errs <- err
		}
		close(errs)
	}()

	select {
	case err := <-errs:
		return err
	case <-ctx.Done():
		slog.Info("停止シグナルを受け取った。処理中のリクエストを待つ")
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("停止に失敗: %w", err)
	}
	return nil
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
