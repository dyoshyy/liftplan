package usecase_test

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/dyoshyy/liftplan/internal/application/apperror"
	"github.com/dyoshyy/liftplan/internal/application/usecase"
	"github.com/dyoshyy/liftplan/internal/domain/account"
	"github.com/dyoshyy/liftplan/internal/domain/training"
	"github.com/dyoshyy/liftplan/internal/domain/training/exercise"
	"github.com/dyoshyy/liftplan/internal/domain/training/program"
	"github.com/dyoshyy/liftplan/internal/domain/training/seed"
)

// exerciseRepo は種目の読み書きを模す、このファイル内だけの疑似リポジトリ。
//
// internal/infrastructure/memory を使わないのは、application 層のテストが
// infrastructure 層を import すると TestOnion_DependenciesPointInward
// （internal/architecture_test.go）が落ちるため。sign_in_test.go の
// 「保存先はこのファイルで組む」と同じやり方に揃える。
//
// memory.ExerciseRepository の重複チェックはあえて持たせない。ここに
// 同じ規則を持たせると、ユースケース側の重複チェックを消しても
// フェイクが代わりに弾いてしまい、変異テストが検出力を失う。
type exerciseRepo struct {
	mu   sync.Mutex
	seed []*exercise.Exercise
	mine map[account.UserID][]*exercise.Exercise
}

func newExerciseRepo(seedAll []*exercise.Exercise) *exerciseRepo {
	return &exerciseRepo{seed: seedAll, mine: map[account.UserID][]*exercise.Exercise{}}
}

// FindAll はシードの後ろに、その利用者が足した種目を足した順で返す。
func (r *exerciseRepo) FindAll(_ context.Context, user account.UserID) ([]*exercise.Exercise, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]*exercise.Exercise, 0, len(r.seed)+len(r.mine[user]))
	out = append(out, r.seed...)
	out = append(out, r.mine[user]...)
	return out, nil
}

// Save は同じ ID を上書きし、無ければ足す。
func (r *exerciseRepo) Save(_ context.Context, user account.UserID, e *exercise.Exercise) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	list := r.mine[user]
	for i, existing := range list {
		if existing.ID() == e.ID() {
			list[i] = e
			return nil
		}
	}
	r.mine[user] = append(list, e)
	return nil
}

// programRepo はプログラムの読み書きを模す、このファイル内だけの疑似リポジトリ。
//
// exerciseRepo と同じ理由で internal/infrastructure/memory を使わない。
type programRepo struct {
	mu     sync.Mutex
	byUser map[account.UserID]*program.Program
}

func newProgramRepo() *programRepo {
	return &programRepo{byUser: map[account.UserID]*program.Program{}}
}

// Get は未設定なら program.ErrProgramNotConfigured を返す。(nil, nil) を
// 返すと、呼び出し側の WithSelected が nil 逆参照で落ち、
// 「プログラム未設定」ではなく別の理由でテストが赤くなる。
func (r *programRepo) Get(_ context.Context, user account.UserID) (*program.Program, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	p, ok := r.byUser[user]
	if !ok {
		return nil, program.ErrProgramNotConfigured
	}
	return p, nil
}

func (r *programRepo) Save(_ context.Context, user account.UserID, p *program.Program) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.byUser[user] = p
	return nil
}

func newAdd(t *testing.T) (*usecase.AddCustomExercise, *exerciseRepo, *programRepo, account.UserID) {
	t.Helper()
	seedAll, err := seed.Exercises()
	if err != nil {
		t.Fatalf("シードが不正: %v", err)
	}
	exercises := newExerciseRepo(seedAll)
	programs := newProgramRepo()
	user := testUser
	prog, err := seed.DefaultProgram(seedAll)
	if err != nil {
		t.Fatalf("デフォルトのプログラムが不正: %v", err)
	}
	if err := programs.Save(context.Background(), user, prog); err != nil {
		t.Fatalf("プログラムの保存に失敗: %v", err)
	}
	return usecase.NewAddCustomExercise(exercises, programs, programs), exercises, programs, user
}

func isoRow() usecase.AddCustomExerciseInput {
	return usecase.AddCustomExerciseInput{
		Name:        "アイソラテラル・ロー",
		Primary:     []training.MuscleRegion{training.TrapMid},
		Secondary:   []training.MuscleRegion{training.Lat, training.Biceps},
		IncrementKg: 2.5,
	}
}

func TestAddCustomExercise_AddsAndSelects(t *testing.T) {
	ctx := context.Background()
	add, exercises, programs, user := newAdd(t)

	e, err := add.Execute(ctx, user, isoRow())
	if err != nil {
		t.Fatal(err)
	}
	all, _ := exercises.FindAll(ctx, user)
	if all[len(all)-1].ID() != e.ID() {
		t.Error("足した種目が一覧に無い")
	}
	prog, _ := programs.Get(ctx, user)
	if !prog.Includes(e.ID()) {
		t.Error("足した種目が使う種目に入っていない")
	}
}

func TestAddCustomExercise_RejectsDuplicateNames(t *testing.T) {
	ctx := context.Background()
	cases := []struct {
		name string
		prep func(*usecase.AddCustomExercise, account.UserID)
		in   string
	}{
		{"共通の種目と同じ", func(*usecase.AddCustomExercise, account.UserID) {}, "サイドレイズ"},
		{"自分の種目と同じ", func(a *usecase.AddCustomExercise, u account.UserID) {
			_, _ = a.Execute(ctx, u, isoRow())
		}, "アイソラテラル・ロー"},
		{"前後の空白だけ違う", func(a *usecase.AddCustomExercise, u account.UserID) {
			_, _ = a.Execute(ctx, u, isoRow())
		}, " アイソラテラル・ロー "},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			add, _, _, user := newAdd(t)
			c.prep(add, user)
			in := isoRow()
			in.Name = c.in
			if _, err := add.Execute(ctx, user, in); !errors.Is(err, apperror.ErrDuplicateName) {
				t.Errorf("通ってしまった: %v", err)
			}
		})
	}
}

func TestAddCustomExercise_InvalidInputIs400(t *testing.T) {
	add, _, _, user := newAdd(t)
	in := isoRow()
	in.Primary = nil
	if _, err := add.Execute(context.Background(), user, in); !errors.Is(err, apperror.ErrInvalidInput) {
		t.Errorf("主なしが ErrInvalidInput にならない: %v", err)
	}
}

func TestAddCustomExercise_RequiresAProgram(t *testing.T) {
	seedAll, err := seed.Exercises()
	if err != nil {
		t.Fatalf("シードが不正: %v", err)
	}
	programs := newProgramRepo()
	add := usecase.NewAddCustomExercise(newExerciseRepo(seedAll), programs, programs)
	_, err = add.Execute(context.Background(), testUser, isoRow())
	if !errors.Is(err, apperror.ErrNotConfigured) {
		t.Errorf("プログラム未設定で通った: %v", err)
	}
}
