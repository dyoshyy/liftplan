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
// ミドルウェアだけで、それが載せるのは常に既定ユーザーなので、
// 「ハンドラが context を無視して account.DefaultUserID() を直接渡す」
// 実装に戻しても外からは区別がつかない。載せた値と既定値が同じだから。
//
// 既定以外の利用者を1人だけ登場させれば区別できる。その人のプログラム
// だけを保存しておき、その人として叩いたときに 200、既定ユーザーとして
// 叩いたときに 404 になることを見る。ハンドラが context を見ていなければ、
// 前者が 404 になって落ちる。
func TestHandler_UsesTheUserFromContext(t *testing.T) {
	userB, err := account.NewUserID("11111111-2222-3333-4444-555555555555")
	if err != nil {
		t.Fatalf("利用者の識別子が不正: %v", err)
	}
	if userB == account.DefaultUserID() {
		t.Fatal("既定ユーザーと同じ値では、届いているかを区別できない")
	}

	routes, programs := routesForUserTest(t)
	// 保存するのは userB のぶんだけ。既定ユーザーには何も無い。
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
			// 既定ユーザーには保存していない。ここが 200 になるなら、
			// ハンドラは context ではなく既定ユーザーを見ている。
			name: "別の利用者からは同じ設定が見えない",
			user: account.DefaultUserID(), want: http.StatusNotFound,
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
		usecase.NewConfigureProgram(exercises, programs),
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
