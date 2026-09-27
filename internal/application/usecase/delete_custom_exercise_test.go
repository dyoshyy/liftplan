package usecase_test

import (
	"context"
	"errors"
	"testing"

	"github.com/dyoshyy/liftplan/internal/application/apperror"
	"github.com/dyoshyy/liftplan/internal/application/query"
	"github.com/dyoshyy/liftplan/internal/application/usecase"
	"github.com/dyoshyy/liftplan/internal/domain/account"
	"github.com/dyoshyy/liftplan/internal/domain/training/exercise"
	"github.com/dyoshyy/liftplan/internal/domain/training/program"
	"github.com/dyoshyy/liftplan/internal/domain/training/setlog"
)

// 消す／足すを同じリポジトリで組む。
type fixture struct {
	add       *usecase.AddCustomExercise
	del       *usecase.DeleteCustomExercise
	exercises *exerciseRepo
	programs  *programRepo
	user      account.UserID
}

func newFixture(t *testing.T) fixture {
	t.Helper()
	add, exercises, programs, user := newAdd(t)
	del := usecase.NewDeleteCustomExercise(exercises, programs, programs)
	return fixture{add: add, del: del, exercises: exercises, programs: programs, user: user}
}

func TestDeleteCustomExercise_DeletesAndUnselects(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	e, err := f.add.Execute(ctx, f.user, isoRow())
	if err != nil {
		t.Fatalf("足すのに失敗: %v", err)
	}

	if err := f.del.Execute(ctx, f.user, e.ID()); err != nil {
		t.Fatal(err)
	}
	all, _ := f.exercises.FindAll(ctx, f.user)
	if got := all[len(all)-1]; got.ID() != e.ID() || !got.IsDeleted() {
		t.Error("消した種目が消えた状態で残っていない")
	}
	prog, _ := f.programs.Get(ctx, f.user)
	if prog.Includes(e.ID()) {
		t.Error("消した種目が使う種目に残っている")
	}
}

// 足す → 消す → もう一度消す が nil であること。
func TestDeleteCustomExercise_IsIdempotent(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	e, err := f.add.Execute(ctx, f.user, isoRow())
	if err != nil {
		t.Fatalf("足すのに失敗: %v", err)
	}
	if err := f.del.Execute(ctx, f.user, e.ID()); err != nil {
		t.Fatalf("1回目の削除に失敗: %v", err)
	}
	if err := f.del.Execute(ctx, f.user, e.ID()); err != nil {
		t.Errorf("2回目の削除が失敗した: %v", err)
	}
}

func TestDeleteCustomExercise_NotFound(t *testing.T) {
	ctx := context.Background()
	for name, id := range map[string]exercise.ExerciseID{
		"共通の種目": "side_raise",
		"存在しない": "u-ffffffffffffffff",
	} {
		t.Run(name, func(t *testing.T) {
			f := newFixture(t)
			if err := f.del.Execute(ctx, f.user, id); !errors.Is(err, apperror.ErrExerciseNotFound) {
				t.Errorf("404 にならない: %v", err)
			}
		})
	}
	t.Run("他人の種目", func(t *testing.T) {
		f := newFixture(t)
		e, err := f.add.Execute(ctx, f.user, isoRow())
		if err != nil {
			t.Fatalf("足すのに失敗: %v", err)
		}
		other := mustUserID(t)
		// other にもプログラムを入れておく（未設定の 409 と混ぜない）
		prog, _ := f.programs.Get(ctx, f.user)
		if err := f.programs.Save(ctx, other, prog); err != nil {
			t.Fatalf("他人のプログラムの保存に失敗: %v", err)
		}
		if err := f.del.Execute(ctx, other, e.ID()); !errors.Is(err, apperror.ErrExerciseNotFound) {
			t.Errorf("他人の種目が 404 にならない: %v", err)
		}
	})
}

func TestDeleteCustomExercise_RefusesDeclared(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	e, err := f.add.Execute(ctx, f.user, isoRow())
	if err != nil {
		t.Fatalf("足すのに失敗: %v", err)
	}
	prog, _ := f.programs.Get(ctx, f.user)
	next, err := prog.WithDeclared(append(prog.DeclaredExercises(), e.ID()))
	if err != nil {
		t.Fatalf("宣言の更新に失敗: %v", err)
	}
	if err := f.programs.Save(ctx, f.user, next); err != nil {
		t.Fatalf("プログラムの保存に失敗: %v", err)
	}

	if err := f.del.Execute(ctx, f.user, e.ID()); !errors.Is(err, apperror.ErrStillDeclared) {
		t.Errorf("伸ばしたい種目が消せた: %v", err)
	}
}

// 消した種目と同じ名前で足し直せること。
func TestAddCustomExercise_AllowsTheNameOfADeletedExercise(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	e, err := f.add.Execute(ctx, f.user, isoRow())
	if err != nil {
		t.Fatalf("足すのに失敗: %v", err)
	}
	if err := f.del.Execute(ctx, f.user, e.ID()); err != nil {
		t.Fatalf("削除に失敗: %v", err)
	}
	if _, err := f.add.Execute(ctx, f.user, isoRow()); err != nil {
		t.Errorf("消した種目と同名が弾かれた: %v", err)
	}
}

// failOnceProgramWriter は program.Writer を埋め込み、最初の Save だけ
// 失敗を返すスタブ。途中で落ちたときに壊れた状態を残さないことを見る。
type failOnceProgramWriter struct {
	program.Writer
	failed bool
}

func (w *failOnceProgramWriter) Save(ctx context.Context, user account.UserID, p *program.Program) error {
	if !w.failed {
		w.failed = true
		return errors.New("一時的に保存できない")
	}
	return w.Writer.Save(ctx, user, p)
}

// 途中で落ちても壊れた状態を残さないこと（Review Focus 1）。
//
// 壊れた状態とは「種目は消えたのに使う種目に残っている」こと。そうなると
// verifySelection が消した種目を弾くので、以後の選択の保存が失敗し続け、
// 設定の一覧にも消した種目は出ないので本人は直せない。
//
// プログラムの保存を1回だけ落とす。書く順が「種目→プログラム」だと、
// 1回目で種目だけが消えてこの状態になる。
func TestDeleteCustomExercise_LeavesNoBrokenStateOnFailure(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	e, err := f.add.Execute(ctx, f.user, isoRow())
	if err != nil {
		t.Fatalf("足すのに失敗: %v", err)
	}

	flaky := &failOnceProgramWriter{Writer: f.programs}
	del := usecase.NewDeleteCustomExercise(f.exercises, f.programs, flaky)
	if err := del.Execute(ctx, f.user, e.ID()); err == nil {
		t.Fatal("1回目が落ちなかった（スタブが効いていない）")
	}

	all, _ := f.exercises.FindAll(ctx, f.user)
	prog, _ := f.programs.Get(ctx, f.user)
	if all[len(all)-1].IsDeleted() && prog.Includes(e.ID()) {
		t.Fatal("種目は消えたのに使う種目に残っている")
	}

	if err := del.Execute(ctx, f.user, e.ID()); err != nil {
		t.Fatalf("消し直せない: %v", err)
	}
	all, _ = f.exercises.FindAll(ctx, f.user)
	prog, _ = f.programs.Get(ctx, f.user)
	if !all[len(all)-1].IsDeleted() || prog.Includes(e.ID()) {
		t.Error("消し直した後の状態が違う")
	}
}

// 消えているのに選択に残っている状態でも、選択から外せること。
//
// target.IsDeleted() を見て早期に return すると、この除去が動かなくなる。
// LeavesNoBrokenStateOnFailure は「種目だけが先に消える」状態を保存の
// 失敗経由でしか作れず、後半の分岐まで届く前に前半の検査で止まるため、
// ここで直接その状態を組み立てて確かめる。
func TestDeleteCustomExercise_RepairsSelectionEvenIfAlreadyDeleted(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	e, err := f.add.Execute(ctx, f.user, isoRow())
	if err != nil {
		t.Fatalf("足すのに失敗: %v", err)
	}
	// 前回、種目の保存だけが先に終わってプログラムの保存が落ちた状態を、
	// 直接組み立てる。
	if err := f.exercises.Save(ctx, f.user, e.Delete()); err != nil {
		t.Fatalf("種目の直接削除に失敗: %v", err)
	}
	prog, _ := f.programs.Get(ctx, f.user)
	if !prog.Includes(e.ID()) {
		t.Fatalf("前提が崩れている: 選択にまだ残っているはず")
	}

	if err := f.del.Execute(ctx, f.user, e.ID()); err != nil {
		t.Fatalf("消し直せない: %v", err)
	}
	prog, _ = f.programs.Get(ctx, f.user)
	if prog.Includes(e.ID()) {
		t.Error("消えているのに選択から外れていない")
	}
}

// recordingLogs は保存した実績をそのまま読み返せる、テスト用の実績
// リポジトリ。record_test.go の fakeLogs は保存 (saved) と読み取り
// (history) が別のフィールドなので、書いてすぐ読み返す
// TestDeletedExercise_KeepsLogsAndNames には使えない。
type recordingLogs struct {
	logs []*setlog.SetLog
}

func (r *recordingLogs) FindAll(context.Context, account.UserID) (setlog.History, error) {
	return setlog.NewHistory(r.logs), nil
}

func (r *recordingLogs) Save(_ context.Context, _ account.UserID, logs []*setlog.SetLog) error {
	r.logs = append(r.logs, logs...)
	return nil
}

func (r *recordingLogs) Delete(context.Context, account.UserID, setlog.SetLogID) error {
	return nil
}

// 消した種目の記録は受け付け、履歴に名前が出ること（Review Focus 4）。
func TestDeletedExercise_KeepsLogsAndNames(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	e, err := f.add.Execute(ctx, f.user, isoRow())
	if err != nil {
		t.Fatalf("足すのに失敗: %v", err)
	}
	if err := f.del.Execute(ctx, f.user, e.ID()); err != nil {
		t.Fatalf("削除に失敗: %v", err)
	}

	logs := &recordingLogs{}
	rec := usecase.NewRecordSets(logs, f.exercises)
	log, err := setlog.NewSetLog(setlog.SetLogParams{
		ID: "01J-DEL", PerformedOn: testDate, ExerciseID: string(e.ID()),
		WeightKg: 40, Reps: 8, RIR: 2,
	})
	if err != nil {
		t.Fatalf("ログ生成に失敗: %v", err)
	}
	if err := rec.Execute(ctx, f.user, []*setlog.SetLog{log}); err != nil {
		t.Fatalf("消した種目の記録が拒否された: %v", err)
	}

	h := query.NewHistory(logs, f.exercises)
	days, err := h.Days(ctx, f.user, testDate, testDate)
	if err != nil {
		t.Fatalf("履歴が読めない: %v", err)
	}
	if len(days) != 1 || len(days[0].Exercises) != 1 {
		t.Fatalf("履歴が拾えない: %+v", days)
	}
	if got := days[0].Exercises[0].Name; got != "アイソラテラル・ロー" {
		t.Errorf("消した種目の名前が履歴に出ない: %q", got)
	}
}
