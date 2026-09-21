package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/dyoshyy/liftplan/internal/application/query"
	"github.com/dyoshyy/liftplan/internal/application/usecase"
	"github.com/dyoshyy/liftplan/internal/domain/account"
	"github.com/dyoshyy/liftplan/internal/domain/training/exercise"
	"github.com/dyoshyy/liftplan/internal/domain/training/planning"
	"github.com/dyoshyy/liftplan/internal/domain/training/program"
	"github.com/dyoshyy/liftplan/internal/domain/training/seed"
	"github.com/dyoshyy/liftplan/internal/infrastructure/memory"
)

// context に載った利用者が、ユースケースまでそのまま届く。
//
// **なぜ外部テストでは書けないか。**鍵の型（userContextKey）は非公開で、
// httpapi の外から利用者を詰められない。外から動かせるのは認証
// ミドルウェアだけで、そこに利用者を供給するのはセッションの店なので、
// 外部テストが確かめられるのは「店が言った人が届くか」までになる。
//
// ここで見たいのはその1つ内側、**ハンドラが context を見ているか**。
// 2人を登場させ、片方のプログラムだけを保存しておく。その人として
// 叩けば 200、もう片方として叩けば 404。ハンドラが context を無視して
// 固定の利用者を渡す実装に戻せば、前者が 404 になって落ちる。
func TestHandler_UsesTheUserFromContext(t *testing.T) {
	userB, err := account.NewUserID("11111111-2222-3333-4444-555555555555")
	if err != nil {
		t.Fatalf("利用者の識別子が不正: %v", err)
	}
	other, err := account.NewUserID("99999999-8888-4777-a666-555555555555")
	if err != nil {
		t.Fatalf("利用者の識別子が不正: %v", err)
	}
	if userB == other {
		t.Fatal("2人が同じ値では、届いているかを区別できない")
	}

	routes, programs := routesForUserTest(t)
	// 保存するのは userB のぶんだけ。もう片方には何も無い。
	if err := programs.Save(context.Background(), userB, someProgram(t)); err != nil {
		t.Fatalf("プログラムの保存に失敗: %v", err)
	}

	cases := []struct {
		name string
		user account.UserID
		want int
	}{
		{
			// 保存したのはこの人のぶん。届いていれば読める。
			name: "context に載せた利用者の設定が読める",
			user: userB, want: http.StatusOK,
		},
		{
			// こちらには保存していない。ここが 200 になるなら、
			// ハンドラは context ではなく固定の利用者を見ている。
			name: "別の利用者からは同じ設定が見えない",
			user: other, want: http.StatusNotFound,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, "/api/program", nil)
			r = r.WithContext(withUser(r.Context(), c.user))
			rec := httptest.NewRecorder()
			routes.ServeHTTP(rec, r)

			if rec.Code != c.want {
				t.Errorf("ステータスが %d。%d のはず: body=%s",
					rec.Code, c.want, rec.Body.String())
			}
		})
	}
}

// routesForUserTest はインメモリのリポジトリでルータを組む。
func routesForUserTest(t *testing.T) (http.Handler, *memory.ProgramRepository) {
	t.Helper()

	pool, err := seed.Exercises()
	if err != nil {
		t.Fatalf("シードが不正: %v", err)
	}
	exercises := memory.NewExerciseRepository(pool)
	logs := memory.NewSetLogRepository()
	conditions := memory.NewConditionRepository()
	programs := memory.NewProgramRepository()

	h := NewHandler(
		usecase.NewGetSession(exercises, logs, conditions, programs, planning.DefaultSessionPlanner()),
		usecase.NewRecordSets(logs, exercises),
		usecase.NewRecordConditions(conditions),
		usecase.NewSetFocusExercise(programs, programs),
		usecase.NewSetDeclaredExercises(programs, programs),
		usecase.NewSetFrequency(programs, programs),
		usecase.NewSetSelectedExercises(exercises, programs, programs),
		usecase.NewSetWeeklyTarget(exercises, programs, programs),
		usecase.NewSetSplitCycle(exercises, programs, programs),
		usecase.NewGetProgram(programs),
		usecase.NewDeleteSetLog(logs),
		query.NewExercises(exercises),
		query.NewHistory(logs, exercises),
		query.NewStats(logs, exercises, programs, planning.DefaultOneRepMaxEstimator()),
	)
	return h.Routes(), programs
}

// someProgram は中身を問わない1つのプログラム。
func someProgram(t *testing.T) *program.Program {
	t.Helper()

	pool, err := seed.Exercises()
	if err != nil {
		t.Fatalf("シードが不正: %v", err)
	}
	freq, err := program.NewFrequency(3)
	if err != nil {
		t.Fatalf("頻度が不正: %v", err)
	}
	target, err := seed.DefaultWeeklyTarget(freq)
	if err != nil {
		t.Fatalf("週目標が不正: %v", err)
	}
	selected := make([]exercise.ExerciseID, 0, len(pool))
	for _, e := range pool {
		selected = append(selected, e.ID())
	}
	prog, err := program.NewProgram(freq, target, selected,
		[]exercise.ExerciseID{"bench", "squat", "deadlift"}, "")
	if err != nil {
		t.Fatalf("プログラムが不正: %v", err)
	}
	return prog
}
