package postgres_test

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/dyoshyy/liftplan/internal/domain/account"
	"github.com/dyoshyy/liftplan/internal/domain/training"
	"github.com/dyoshyy/liftplan/internal/domain/training/exercise"
	"github.com/dyoshyy/liftplan/internal/domain/training/seed"
	"github.com/dyoshyy/liftplan/internal/infrastructure/postgres"
)

// newUser はテスト用の利用者ID。呼ぶたびに別人になる。
func newUser(t *testing.T) account.UserID {
	t.Helper()
	id, err := account.NewRandomUserID()
	if err != nil {
		t.Fatalf("UserID が作れない: %v", err)
	}
	return id
}

func mustCustom(t *testing.T, id, name string) *exercise.Exercise {
	t.Helper()
	e, err := exercise.NewCustomExercise(exercise.CustomExerciseParams{
		ID: id, Name: name,
		Primary: []training.MuscleRegion{training.Lat}, IncrementKg: 2.5,
	})
	if err != nil {
		t.Fatal(err)
	}
	return e
}

func TestExerciseRepository_SavesAndReadsBack(t *testing.T) {
	ctx := context.Background()
	seedAll, _ := seed.Exercises()
	repo := postgres.NewExerciseRepository(migratedDB(t), seedAll)

	e, err := exercise.NewCustomExercise(exercise.CustomExerciseParams{
		ID: "u-000000000000000a", Name: "アイソラテラル・ロー",
		Primary:     []training.MuscleRegion{training.TrapMid},
		Secondary:   []training.MuscleRegion{training.Lat, training.Biceps},
		IncrementKg: 2.5,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.Save(ctx, userA(t), e); err != nil {
		t.Fatal(err)
	}

	got, err := repo.FindAll(ctx, userA(t))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != len(seedAll)+1 {
		t.Fatalf("%d 件（期待 %d）", len(got), len(seedAll)+1)
	}
	back := got[len(got)-1]
	if back.ID() != e.ID() || back.Name() != e.Name() || !back.IsCustom() || back.IsDeleted() ||
		back.Increment().Kg() != 2.5 ||
		!slices.Equal(back.PrimaryRegions(), e.PrimaryRegions()) ||
		!slices.Equal(back.SecondaryRegions(), e.SecondaryRegions()) {
		t.Errorf("読み戻した種目が違う: %+v", back)
	}
}

// 消すのは論理削除。消した後も1件残り、IsDeleted() が立つこと。
func TestExerciseRepository_KeepsDeletedCustoms(t *testing.T) {
	ctx := context.Background()
	seedAll, _ := seed.Exercises()
	repo := postgres.NewExerciseRepository(migratedDB(t), seedAll)
	a := newUser(t)
	e := mustCustom(t, "u-000000000000000a", "アイソラテラル・ロー")
	if err := repo.Save(ctx, a, e); err != nil {
		t.Fatal(err)
	}
	if err := repo.Save(ctx, a, e.Delete()); err != nil {
		t.Fatal(err)
	}

	got, err := repo.FindAll(ctx, a)
	if err != nil {
		t.Fatal(err)
	}
	custom := got[len(got)-1]
	if len(got) != len(seedAll)+1 || !custom.IsDeleted() {
		t.Errorf("消した種目が消えた状態で1件残っていない: %+v", custom)
	}
}

func TestExerciseRepository_RefusesSeedExercises(t *testing.T) {
	seedAll, _ := seed.Exercises()
	repo := postgres.NewExerciseRepository(migratedDB(t), seedAll)
	if err := repo.Save(context.Background(), newUser(t), seedAll[0]); err == nil {
		t.Error("共通の種目を保存できてしまった")
	}
}

// 消していない同じ名前は弾き、消した種目と同じ名前は通す（DB の部分一意
// 索引と同じふるまい）。memory 側の同名テストと同じ3手順。
func TestExerciseRepository_NameIsUniqueAmongAliveCustoms(t *testing.T) {
	ctx := context.Background()
	seedAll, _ := seed.Exercises()
	repo := postgres.NewExerciseRepository(migratedDB(t), seedAll)
	a := newUser(t)
	first := mustCustom(t, "u-000000000000000a", "アイソラテラル・ロー")
	if err := repo.Save(ctx, a, first); err != nil {
		t.Fatal(err)
	}

	dup := mustCustom(t, "u-000000000000000b", "アイソラテラル・ロー")
	if err := repo.Save(ctx, a, dup); !errors.Is(err, exercise.ErrDuplicateExerciseName) {
		t.Errorf("同名が通った: %v", err)
	}
	if err := repo.Save(ctx, a, first.Delete()); err != nil {
		t.Fatal(err)
	}
	if err := repo.Save(ctx, a, dup); err != nil {
		t.Errorf("消した種目と同名が弾かれた: %v", err)
	}
}

// 保存した順序に関わらず、FindAll はシードの後ろに自分の種目を ID の
// 昇順で並べて返す。ID の降順で保存しても結果は昇順になること。
func TestExerciseRepository_OrdersCustomsByIDRegardlessOfSaveOrder(t *testing.T) {
	ctx := context.Background()
	seedAll, _ := seed.Exercises()
	repo := postgres.NewExerciseRepository(migratedDB(t), seedAll)
	a := newUser(t)

	second := mustCustom(t, "u-000000000000000b", "アイソラテラル・ロー")
	first := mustCustom(t, "u-000000000000000a", "シーテッドロー")
	// b を先に、a を後に保存する（ID の降順）。
	if err := repo.Save(ctx, a, second); err != nil {
		t.Fatal(err)
	}
	if err := repo.Save(ctx, a, first); err != nil {
		t.Fatal(err)
	}

	got, err := repo.FindAll(ctx, a)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != len(seedAll)+2 {
		t.Fatalf("%d 件（期待 %d）", len(got), len(seedAll)+2)
	}
	tail := got[len(seedAll):]
	if tail[0].ID() != first.ID() || tail[1].ID() != second.ID() {
		t.Errorf("保存順のまま返っている（ID 昇順のはず）: %v, %v", tail[0].ID(), tail[1].ID())
	}
}

// 同じ ID を二度渡したら上書きする（exercise.Writer の契約）。
// 名前・部位・刻みも含めて、2回目の値だけが残ること。
func TestExerciseRepository_SaveOverwritesTheSameID(t *testing.T) {
	ctx := context.Background()
	seedAll, _ := seed.Exercises()
	repo := postgres.NewExerciseRepository(migratedDB(t), seedAll)
	a := newUser(t)

	first, err := exercise.NewCustomExercise(exercise.CustomExerciseParams{
		ID: "u-000000000000000a", Name: "アイソラテラル・ロー",
		Primary: []training.MuscleRegion{training.Lat}, IncrementKg: 2.5,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.Save(ctx, a, first); err != nil {
		t.Fatal(err)
	}

	second, err := exercise.NewCustomExercise(exercise.CustomExerciseParams{
		ID: "u-000000000000000a", Name: "シーテッドロー",
		Primary: []training.MuscleRegion{training.TrapMid}, IncrementKg: 5.0,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.Save(ctx, a, second); err != nil {
		t.Fatal(err)
	}

	got, err := repo.FindAll(ctx, a)
	if err != nil {
		t.Fatal(err)
	}
	back := got[len(got)-1]
	if back.Name() != second.Name() || back.Increment().Kg() != second.Increment().Kg() ||
		!slices.Equal(back.PrimaryRegions(), second.PrimaryRegions()) {
		t.Errorf("2回目の値で上書きされていない: %+v", back)
	}
}
