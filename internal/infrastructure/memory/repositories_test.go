package memory_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/dyoshyy/liftplan/internal/domain/training"
	"github.com/dyoshyy/liftplan/internal/domain/training/exercise"
	"github.com/dyoshyy/liftplan/internal/domain/training/seed"
	"github.com/dyoshyy/liftplan/internal/infrastructure/memory"

	"github.com/dyoshyy/liftplan/internal/domain/training/condition"
	"github.com/dyoshyy/liftplan/internal/domain/training/setlog"
)

var day = training.MustDate(2026, time.August, 17)

func mustCustom(t *testing.T, id, name string) *exercise.Exercise {
	t.Helper()
	e, err := exercise.NewExercise(exercise.ExerciseParams{
		ID: id, Name: name,
		Stimulus:    map[training.MuscleRegion]float64{training.Lat: 1.0},
		IncrementKg: 2.5,
	})
	if err != nil {
		t.Fatal(err)
	}
	return e
}

// findByID はテストが特定の種目を確かめるための探索。
//
// FindAll の並びは全体で ID 昇順なので、末尾や先頭が目的の種目とは限らない。
// 位置ではなく ID で探すことで、他の種目の有無や並びが変わってもテストの
// 意図がぶれない。
func findByID(list []*exercise.Exercise, id exercise.ExerciseID) *exercise.Exercise {
	for _, e := range list {
		if e.ID() == id {
			return e
		}
	}
	return nil
}

// stimulusMapOf は Edit の入力を組み立てるために、既存の種目から
// 効き方をそのまま取り出す。テストが検証したいのは名前や刻みの上書きで、
// 効き方を変える意図ではない。
func stimulusMapOf(e *exercise.Exercise) map[training.MuscleRegion]float64 {
	m := make(map[training.MuscleRegion]float64)
	for _, r := range e.Stimulus().Regions() {
		c, _ := e.Stimulus().Contribution(r)
		m[r] = c.Float()
	}
	return m
}

func mkSetLog(t *testing.T, id string, kg float64) *setlog.SetLog {
	t.Helper()
	l, err := setlog.NewSetLog(setlog.SetLogParams{
		ID: id, PerformedOn: day, ExerciseID: "bench",
		WeightKg: kg, Reps: 9, RIR: 2,
	})
	if err != nil {
		t.Fatalf("ログ生成に失敗: %v", err)
	}
	return l
}

// FindAll が返す値がリポジトリ内部の状態をエイリアスしないこと。
// TestExerciseRepository_SeedsOnFirstRead が件数と ID 集合を確かめるので、
// ここでは「呼び出し側がスライスを壊しても内部状態は壊れない」ことだけを見る。
func TestExerciseRepository_DoesNotAliasItsState(t *testing.T) {
	pool, err := seed.Exercises()
	if err != nil {
		t.Fatalf("シードが不正: %v", err)
	}
	repo := memory.NewExerciseRepository(pool)
	a := newUser(t)

	// 呼び出し側がスライスを壊しても、次の取得に影響しない。
	got, _ := repo.FindAll(context.Background(), a)
	for i := range got {
		got[i] = nil
	}

	again, _ := repo.FindAll(context.Background(), a)
	for i, e := range again {
		if e == nil {
			t.Fatalf("%d番目が nil になっている", i)
		}
	}

	// コンストラクタに渡したスライスを後から壊しても影響しない。
	// 別の利用者で確かめる。既に読んだ利用者はシードから map を作り終えて
	// おり、以後 r.seed を読まないため、同じ利用者では pool を壊しても
	// 見た目上テストが通ってしまう（エイリアスしていなくても green になる）。
	pool[0] = nil
	b := newUser(t)
	third, err := repo.FindAll(context.Background(), b)
	if err != nil {
		t.Fatalf("取得に失敗: %v", err)
	}
	if len(third) != len(pool) {
		t.Fatalf("件数が誤り: got %d, want %d", len(third), len(pool))
	}
	for i, e := range third {
		if e == nil {
			t.Fatalf("コンストラクタの引数をエイリアスしている（%d番目が nil）", i)
		}
	}
}

// Save と FindAll を並行に呼んでも壊れないこと。
func TestRepositories_AreSafeForConcurrentUse(t *testing.T) {
	logs := memory.NewSetLogRepository()
	conditions := memory.NewConditionRepository()
	programs := memory.NewProgramRepository()
	ctx := context.Background()
	user := userA(t)

	seedAll, err := seed.Exercises()
	if err != nil {
		t.Fatalf("シードが不正: %v", err)
	}
	// 種目は専用の利用者にする。FindAll の「無ければ入れる」がロックで
	// 守られているかを見るので、他の並行呼び出しと同じ人を使うと、
	// SetLog/Condition の書き込みと衝突が紛れて何が壊れたのか読めない。
	exercises := memory.NewExerciseRepository(seedAll)
	exerciseUser := userB(t)

	var wg sync.WaitGroup
	for i := range 16 {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_ = logs.Save(ctx, user, []*setlog.SetLog{mkSetLog(t, fmt.Sprintf("c%03d", i), 85)})
			_, _ = logs.FindAll(ctx, user)
			_ = conditions.Save(ctx, user, []condition.DailyCondition{
				condition.NewDailyCondition(day.AddDays(-i)).WithBodyWeight(75),
			})
			_, _ = conditions.FindAll(ctx, user)
			_, _ = programs.Get(ctx, user)

			custom := mustCustom(t, fmt.Sprintf("u-0000000000000%03x", i), fmt.Sprintf("並行種目%02d", i))
			_ = exercises.Save(ctx, exerciseUser, custom)
			_, _ = exercises.FindAll(ctx, exerciseUser)
		}(i)
	}
	wg.Wait()

	if logs.Size(user) != 16 {
		t.Errorf("並行保存で件数が合わない: %d", logs.Size(user))
	}
	if conditions.Size(user) != 16 {
		t.Errorf("並行保存で件数が合わない: %d", conditions.Size(user))
	}
	got, err := exercises.FindAll(ctx, exerciseUser)
	if err != nil {
		t.Fatalf("種目の取得に失敗: %v", err)
	}
	if len(got) != len(seedAll)+16 {
		t.Errorf("並行保存で種目の件数が合わない: got %d, want %d", len(got), len(seedAll)+16)
	}
}

// 新しい利用者の最初の FindAll は、シードと同じ件数・同じ ID 集合を返す。
// その人の map がまだ無いので、シードを全部コピーして入れる。
func TestExerciseRepository_SeedsOnFirstRead(t *testing.T) {
	ctx := context.Background()
	seedAll, err := seed.Exercises()
	if err != nil {
		t.Fatalf("シードが不正: %v", err)
	}
	repo := memory.NewExerciseRepository(seedAll)
	a := newUser(t)

	got, err := repo.FindAll(ctx, a)
	if err != nil {
		t.Fatalf("取得に失敗: %v", err)
	}
	if len(got) != len(seedAll) {
		t.Fatalf("件数が誤り: got %d, want %d", len(got), len(seedAll))
	}
	gotIDs := make(map[exercise.ExerciseID]bool, len(got))
	for _, e := range got {
		gotIDs[e.ID()] = true
	}
	for _, e := range seedAll {
		if !gotIDs[e.ID()] {
			t.Errorf("シードの %s が無い", e.ID())
		}
	}
}

// 全部消してから保存しても、次の FindAll は消した件数のまま。
// ensureSeeded は map の有無で判定する（件数では判定しない）ので、全消しの
// 直後にまたシードが入り直り消した記録が生き返る、ということが起きない
// （docs/specs/2026-09-26-custom-exercises-design.md「いつコピーするか」）。
func TestExerciseRepository_DoesNotReseedAfterDeletingAll(t *testing.T) {
	ctx := context.Background()
	seedAll, err := seed.Exercises()
	if err != nil {
		t.Fatalf("シードが不正: %v", err)
	}
	repo := memory.NewExerciseRepository(seedAll)
	a := newUser(t)

	all, err := repo.FindAll(ctx, a)
	if err != nil {
		t.Fatalf("取得に失敗: %v", err)
	}
	for _, e := range all {
		if err := repo.Save(ctx, a, e.Delete()); err != nil {
			t.Fatalf("%s の削除の保存に失敗: %v", e.ID(), err)
		}
	}

	got, err := repo.FindAll(ctx, a)
	if err != nil {
		t.Fatalf("取得に失敗: %v", err)
	}
	if len(got) != len(seedAll) {
		t.Fatalf("件数が変わった: got %d, want %d", len(got), len(seedAll))
	}
	for _, e := range got {
		if !e.IsDeleted() {
			t.Errorf("%s が消えた状態のままになっていない", e.ID())
		}
	}
}

func TestExerciseRepository_KeepsDeletedCustoms(t *testing.T) {
	ctx := context.Background()
	seedAll, err := seed.Exercises()
	if err != nil {
		t.Fatalf("シードが不正: %v", err)
	}
	repo := memory.NewExerciseRepository(seedAll)
	a := newUser(t)
	e := mustCustom(t, "u-000000000000000a", "アイソラテラル・ロー")
	_ = repo.Save(ctx, a, e)
	_ = repo.Save(ctx, a, e.Delete())

	got, _ := repo.FindAll(ctx, a)
	if len(got) != len(seedAll)+1 {
		t.Fatalf("件数が誤り: got %d, want %d", len(got), len(seedAll)+1)
	}
	mine := findByID(got, e.ID())
	if mine == nil || !mine.IsDeleted() {
		t.Errorf("消した種目が消えた状態で1件残っていない: %v", got)
	}
}

// シードの種目を Edit して保存すると、FindAll は直した値を返す。
// プリセット由来かどうかで Save の扱いを変えない。
func TestExerciseRepository_SavesAnyExercise(t *testing.T) {
	ctx := context.Background()
	seedAll, err := seed.Exercises()
	if err != nil {
		t.Fatalf("シードが不正: %v", err)
	}
	repo := memory.NewExerciseRepository(seedAll)
	a := newUser(t)

	original := seedAll[0]
	edited, err := original.Edit(exercise.ExerciseEdit{
		Name:        "改名した" + original.Name(),
		Stimulus:    stimulusMapOf(original),
		IncrementKg: original.Increment().Kg(),
	})
	if err != nil {
		t.Fatalf("Edit に失敗: %v", err)
	}

	if err := repo.Save(ctx, a, edited); err != nil {
		t.Fatalf("プリセット由来の種目を保存できない: %v", err)
	}

	got, err := repo.FindAll(ctx, a)
	if err != nil {
		t.Fatalf("取得に失敗: %v", err)
	}
	if len(got) != len(seedAll) {
		t.Fatalf("件数が変わった: got %d, want %d", len(got), len(seedAll))
	}
	for _, e := range got {
		if e.ID() == original.ID() {
			if e.Name() != edited.Name() {
				t.Errorf("直した名前が返らない: got %q, want %q", e.Name(), edited.Name())
			}
			return
		}
	}
	t.Fatalf("%s が見つからない", original.ID())
}

// 同じ種目を名前を変えずに Edit して再保存しても、自分自身との重複として
// 弾かれないこと。重複チェックが自分の ID を除外し損なうと、名前を
// 変えていない Save は全部 409 になる（Task 3 が Postgres 側に同名のケースを
// 用意する）。
func TestExerciseRepository_SaveKeepsItsOwnName(t *testing.T) {
	ctx := context.Background()
	repo := memory.NewExerciseRepository(nil)
	a := newUser(t)

	original := mustCustom(t, "u-000000000000000a", "アイソラテラル・ロー")
	if err := repo.Save(ctx, a, original); err != nil {
		t.Fatalf("最初の保存に失敗: %v", err)
	}

	edited, err := original.Edit(exercise.ExerciseEdit{
		Name:        original.Name(),
		Stimulus:    stimulusMapOf(original),
		IncrementKg: original.Increment().Kg() + 2.5,
	})
	if err != nil {
		t.Fatalf("Edit に失敗: %v", err)
	}

	if err := repo.Save(ctx, a, edited); err != nil {
		t.Errorf("名前を変えていない自分自身の再保存が弾かれた: %v", err)
	}

	got, err := repo.FindAll(ctx, a)
	if err != nil {
		t.Fatalf("取得に失敗: %v", err)
	}
	back := findByID(got, original.ID())
	if back == nil {
		t.Fatalf("%s が見つからない", original.ID())
	}
	if back.Increment().Kg() != edited.Increment().Kg() {
		t.Errorf("刻みが更新されていない: got %v, want %v", back.Increment().Kg(), edited.Increment().Kg())
	}
}

// 消していない同じ名前は弾き、消した種目と同じ名前は通す（DB の部分一意
// 索引と同じふるまい）。比べる相手はシードも含む
// （docs/specs/2026-09-26-custom-exercises-design.md「消していない種目の中で重複しない」）。
func TestExerciseRepository_NameIsUniqueAmongAliveCustoms(t *testing.T) {
	ctx := context.Background()
	seedAll, err := seed.Exercises()
	if err != nil {
		t.Fatalf("シードが不正: %v", err)
	}
	repo := memory.NewExerciseRepository(seedAll)
	a := newUser(t)
	first := mustCustom(t, "u-000000000000000a", "アイソラテラル・ロー")
	_ = repo.Save(ctx, a, first)

	dup := mustCustom(t, "u-000000000000000b", "アイソラテラル・ロー")
	if err := repo.Save(ctx, a, dup); !errors.Is(err, exercise.ErrDuplicateExerciseName) {
		t.Errorf("同名が通った: %v", err)
	}
	_ = repo.Save(ctx, a, first.Delete())
	if err := repo.Save(ctx, a, dup); err != nil {
		t.Errorf("消した種目と同名が弾かれた: %v", err)
	}

	sameAsSeed := mustCustom(t, "u-000000000000000c", seedAll[0].Name())
	if err := repo.Save(ctx, a, sameAsSeed); !errors.Is(err, exercise.ErrDuplicateExerciseName) {
		t.Errorf("シードと同名が通った: %v", err)
	}
}

// 保存した順序に関わらず、FindAll は全体を ID の昇順で並べて返す。
// ID の降順で保存しても結果は昇順になること。
func TestExerciseRepository_OrdersByID(t *testing.T) {
	ctx := context.Background()
	seedAll, err := seed.Exercises()
	if err != nil {
		t.Fatalf("シードが不正: %v", err)
	}
	repo := memory.NewExerciseRepository(seedAll)
	a := newUser(t)

	second := mustCustom(t, "u-000000000000000b", "アイソラテラル・ロー")
	first := mustCustom(t, "u-000000000000000a", "シーテッドロー2")
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
	for i := 1; i < len(got); i++ {
		if got[i-1].ID() >= got[i].ID() {
			t.Fatalf("ID 昇順になっていない: %d番目 %s, %d番目 %s",
				i-1, got[i-1].ID(), i, got[i].ID())
		}
	}
	tail := got[len(seedAll):]
	if tail[0].ID() != first.ID() || tail[1].ID() != second.ID() {
		t.Errorf("保存順のまま返っている（ID 昇順のはず）: %v, %v", tail[0].ID(), tail[1].ID())
	}
}

// 同じ ID を二度渡したら上書きする（exercise.Writer の契約）。
// 名前・効き方・刻みも含めて、2回目の値だけが残ること。
func TestExerciseRepository_SaveOverwritesTheSameID(t *testing.T) {
	ctx := context.Background()
	seedAll, err := seed.Exercises()
	if err != nil {
		t.Fatalf("シードが不正: %v", err)
	}
	repo := memory.NewExerciseRepository(seedAll)
	a := newUser(t)

	first, err := exercise.NewExercise(exercise.ExerciseParams{
		ID: "u-000000000000000a", Name: "アイソラテラル・ロー",
		Stimulus:    map[training.MuscleRegion]float64{training.Lat: 1.0},
		IncrementKg: 2.5,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.Save(ctx, a, first); err != nil {
		t.Fatal(err)
	}

	second, err := exercise.NewExercise(exercise.ExerciseParams{
		ID: "u-000000000000000a", Name: "シーテッドロー2",
		Stimulus:    map[training.MuscleRegion]float64{training.TrapMid: 1.0},
		IncrementKg: 5.0,
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
	if len(got) != len(seedAll)+1 {
		t.Fatalf("件数が誤り（上書きのはずが増えている）: got %d, want %d", len(got), len(seedAll)+1)
	}
	back := findByID(got, second.ID())
	if back == nil {
		t.Fatalf("%s が見つからない", second.ID())
	}
	if back.Name() != second.Name() || back.Increment().Kg() != second.Increment().Kg() {
		t.Errorf("2回目の値で上書きされていない: %+v", back)
	}
	if c, ok := back.Stimulus().Contribution(training.TrapMid); !ok || c.Float() != 1.0 {
		t.Errorf("2回目の効き方（TrapMid）が反映されていない: %+v", back)
	}
	if _, ok := back.Stimulus().Contribution(training.Lat); ok {
		t.Errorf("1回目の効き方（Lat）が残っている: %+v", back)
	}
}
