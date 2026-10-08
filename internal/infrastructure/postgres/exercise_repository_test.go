package postgres_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/dyoshyy/liftplan/internal/domain/account"
	"github.com/dyoshyy/liftplan/internal/domain/training"
	"github.com/dyoshyy/liftplan/internal/domain/training/exercise"
	"github.com/dyoshyy/liftplan/internal/domain/training/seed"
	"github.com/dyoshyy/liftplan/internal/infrastructure/postgres"
)

// warmPool は n本の接続をあらかじめ張っておく。
//
// 新しいプールは接続を持たずに始まる（遅延接続）。並行テストが2つの
// goroutine を同時に始めても、どちらかが接続を新しく張る分だけ遅れると、
// 狙った競合（count(*) の時点で両方が0件を見る）が再現しない。
func warmPool(ctx context.Context, t *testing.T, pool *pgxpool.Pool, n int) {
	t.Helper()
	var wg sync.WaitGroup
	for range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			conn, err := pool.Acquire(ctx)
			if err != nil {
				t.Errorf("プールを温められない: %v", err)
				return
			}
			conn.Release()
		}()
	}
	wg.Wait()
}

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

// findByID はテストが特定の種目を確かめるための探索。位置ではなく ID で
// 探すことで、他の種目の有無や並びが変わってもテストの意図がぶれない
// （memory 側の同名ヘルパーと同じ理由）。
func findByID(list []*exercise.Exercise, id exercise.ExerciseID) *exercise.Exercise {
	for _, e := range list {
		if e.ID() == id {
			return e
		}
	}
	return nil
}

// stimulusMapOf は Edit の入力を組み立てるために、既存の種目から
// 効き方をそのまま取り出す。
func stimulusMapOf(e *exercise.Exercise) map[training.MuscleRegion]float64 {
	m := make(map[training.MuscleRegion]float64)
	for _, r := range e.Stimulus().Regions() {
		c, _ := e.Stimulus().Contribution(r)
		m[r] = c.Float()
	}
	return m
}

// 新しい利用者の最初の FindAll は、シードと同じ件数・同じ ID 集合を返す。
// その人の行がまだ無いので、シードを1つのトランザクションで全部コピーする。
func TestExerciseRepository_SeedsOnFirstRead(t *testing.T) {
	ctx := context.Background()
	seedAll, err := seed.Exercises()
	if err != nil {
		t.Fatalf("シードが不正: %v", err)
	}
	repo := postgres.NewExerciseRepository(migratedDB(t), seedAll)
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
// ensureSeeded の count(*) は消した行も数えるので、全消しの直後に
// またシードが入り直り消した記録が生き返る、ということが起きない
// （docs/specs/2026-09-26-custom-exercises-design.md「いつコピーするか」）。
func TestExerciseRepository_DoesNotReseedAfterDeletingAll(t *testing.T) {
	ctx := context.Background()
	seedAll, err := seed.Exercises()
	if err != nil {
		t.Fatalf("シードが不正: %v", err)
	}
	repo := postgres.NewExerciseRepository(migratedDB(t), seedAll)
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

// シードの種目を Edit して保存すると、FindAll は直した値を返す。
// プリセット由来かどうかで Save の扱いを変えない。
func TestExerciseRepository_SavesAnyExercise(t *testing.T) {
	ctx := context.Background()
	seedAll, err := seed.Exercises()
	if err != nil {
		t.Fatalf("シードが不正: %v", err)
	}
	repo := postgres.NewExerciseRepository(migratedDB(t), seedAll)
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
	back := findByID(got, original.ID())
	if back == nil {
		t.Fatalf("%s が見つからない", original.ID())
	}
	if back.Name() != edited.Name() {
		t.Errorf("直した名前が返らない: got %q, want %q", back.Name(), edited.Name())
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
	if len(got) != len(seedAll)+1 {
		t.Fatalf("件数が誤り: got %d, want %d", len(got), len(seedAll)+1)
	}
	mine := findByID(got, e.ID())
	if mine == nil || !mine.IsDeleted() {
		t.Errorf("消した種目が消えた状態で1件残っていない: %v", got)
	}
}

// 消していない同じ名前は弾き、消した種目と同じ名前は通す（DB の部分一意
// 索引と同じふるまい）。比べる相手はシードも含む。
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

	sameAsSeed := mustCustom(t, "u-000000000000000c", seedAll[0].Name())
	if err := repo.Save(ctx, a, sameAsSeed); !errors.Is(err, exercise.ErrDuplicateExerciseName) {
		t.Errorf("シードと同名が通った: %v", err)
	}
}

// 保存した順序に関わらず、FindAll は全体を ID の昇順で並べて返す。
// ID の降順で保存しても結果は昇順になること。
func TestExerciseRepository_OrdersByID(t *testing.T) {
	ctx := context.Background()
	seedAll, _ := seed.Exercises()
	repo := postgres.NewExerciseRepository(migratedDB(t), seedAll)
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
}

// 同じ ID を二度渡したら上書きする（exercise.Writer の契約）。
// 名前・効き方・刻みも含めて、2回目の値だけが残ること。
func TestExerciseRepository_SaveOverwritesTheSameID(t *testing.T) {
	ctx := context.Background()
	seedAll, _ := seed.Exercises()
	repo := postgres.NewExerciseRepository(migratedDB(t), seedAll)
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

// 同じ種目を名前を変えずに再保存しても、自分自身との重複として弾かれない
// こと。重複チェックが自分の ID を除外し損なうと、名前を変えていない
// Save は全部 409 になる。
func TestExerciseRepository_SaveKeepsItsOwnName(t *testing.T) {
	ctx := context.Background()
	seedAll, _ := seed.Exercises()
	repo := postgres.NewExerciseRepository(migratedDB(t), seedAll)
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

// 同じ新しい利用者で FindAll を2つの goroutine から同時に呼んでも、
// どちらもエラー無しで、件数はシードと同じになること。
//
// count(*) → シード投入 → SELECT の間に競合が起きると、片方が重複エラーで
// 落ちるか、シードが二重に入って件数が増える。ON CONFLICT DO NOTHING が
// 守る。
//
// プールに接続が1本も無い状態で始めると、片方が新しい接続を張る分だけ
// 遅れて開始し、count(*) の時点で重ならずに競合が起きない（先に始めた方が
// 先に commit を終えている）。事前に複数本の接続を温めておき、2つの
// goroutine が実際に同時に count(*) へ辿り着くようにする。
func TestExerciseRepository_Postgres_ConcurrentFirstReadsSeedOnce(t *testing.T) {
	ctx := context.Background()
	seedAll, err := seed.Exercises()
	if err != nil {
		t.Fatalf("シードが不正: %v", err)
	}
	pool := migratedDB(t)
	repo := postgres.NewExerciseRepository(pool, seedAll)
	a := newUser(t)
	// 旧版の行も1つ置く。取り込みの INSERT も同時に2回走るので、そちらの
	// ON CONFLICT DO NOTHING も守る。
	if _, err := pool.Exec(ctx, `
		INSERT INTO custom_exercises (user_id, id, name, primary_regions, secondary_regions, increment_kg)
		VALUES ($1, 'u-00000000000000cc', '旧版マシン', '["LAT"]', '[]', 2.5)`, a.String()); err != nil {
		t.Fatalf("旧版の行を入れられない: %v", err)
	}
	warmPool(ctx, t, pool, 2)

	var wg sync.WaitGroup
	results := make([][]*exercise.Exercise, 2)
	errs := make([]error, 2)
	start := make(chan struct{})
	for i := range 2 {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			results[i], errs[i] = repo.FindAll(ctx, a)
		}(i)
	}
	close(start)
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Fatalf("%d番目の取得が失敗: %v", i, err)
		}
	}
	for i, got := range results {
		if len(got) != len(seedAll)+1 {
			t.Errorf("%d番目の件数が誤り: got %d, want %d（プリセット＋旧版1件）", i, len(got), len(seedAll)+1)
		}
	}
}

// シードの pull_up（自重係数0.95）と close_grip_bench（派生元 bench）が、
// 読み戻しでも保たれること。
func TestExerciseRepository_Postgres_RoundTripsBodyweightAndDerivedFrom(t *testing.T) {
	ctx := context.Background()
	seedAll, err := seed.Exercises()
	if err != nil {
		t.Fatalf("シードが不正: %v", err)
	}
	repo := postgres.NewExerciseRepository(migratedDB(t), seedAll)
	a := newUser(t)

	got, err := repo.FindAll(ctx, a)
	if err != nil {
		t.Fatalf("取得に失敗: %v", err)
	}

	pullUp := findByID(got, "pull_up")
	if pullUp == nil {
		t.Fatal("シードに pull_up が無い")
	}
	if pullUp.BodyweightFactor().Float() != 0.95 {
		t.Errorf("pull_up の自重係数が %v（期待 0.95）", pullUp.BodyweightFactor().Float())
	}

	closeGripBench := findByID(got, "close_grip_bench")
	if closeGripBench == nil {
		t.Fatal("シードに close_grip_bench が無い")
	}
	from, ok := closeGripBench.DerivedFrom()
	if !ok || from != "bench" {
		t.Errorf("close_grip_bench の派生元が %v, %v（期待 bench, true）", from, ok)
	}
}

// 旧版（#220 初版、0013）で足した種目は、初回の読み出しでプリセットと一緒に
// 取り込むこと。
//
// 旧版は本番に出て、custom_exercises に「主に効く・少し効く」の区分を
// 持っている。取り込まないと、新版に上げた瞬間に足した種目が一覧から消え、
// 記録は名前の無い ID を指すことになる（履歴に名前が出ない）。
func TestExerciseRepository_Postgres_ImportsLegacyCustomExercises(t *testing.T) {
	ctx := context.Background()
	seedAll, err := seed.Exercises()
	if err != nil {
		t.Fatalf("シードが不正: %v", err)
	}
	pool := migratedDB(t)
	repo := postgres.NewExerciseRepository(pool, seedAll)
	a, b := newUser(t), newUser(t)

	// 旧版が書いた形そのまま。消した行も1つ入れる。足した時刻と消した時刻は
	// 固定の値にして、取り込み後に同じ値が残っているかを見る。
	created := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	deletedAt := time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC)
	for _, row := range []struct {
		user               account.UserID
		id, name           string
		primary, secondary string
		deleted            bool
	}{
		{a, "u-0000000000000001", "アイソラテラル・ロー", `["TRAP_MID"]`, `["BICEPS","LAT"]`, false},
		{a, "u-0000000000000002", "消したマシン", `["LAT"]`, `[]`, true},
		{b, "u-0000000000000003", "他人のマシン", `["LAT"]`, `[]`, false},
	} {
		if _, err := pool.Exec(ctx, `
			INSERT INTO custom_exercises
				(user_id, id, name, primary_regions, secondary_regions, increment_kg, created_at, deleted_at)
			VALUES ($1, $2, $3, $4, $5, 2.5, $6, CASE WHEN $7 THEN $8::timestamptz END)`,
			row.user.String(), row.id, row.name, row.primary, row.secondary, created, row.deleted, deletedAt); err != nil {
			t.Fatalf("旧版の行を入れられない: %v", err)
		}
	}

	got, err := repo.FindAll(ctx, a)
	if err != nil {
		t.Fatalf("取得に失敗: %v", err)
	}
	if len(got) != len(seedAll)+2 {
		t.Fatalf("件数が %d（期待 %d：プリセット＋旧版の2件。他人の行は入らない）", len(got), len(seedAll)+2)
	}
	byID := map[exercise.ExerciseID]*exercise.Exercise{}
	for _, e := range got {
		byID[e.ID()] = e
	}
	row := byID["u-0000000000000001"]
	if row == nil || row.Name() != "アイソラテラル・ロー" || row.IsDeleted() || row.Increment().Kg() != 2.5 {
		t.Fatalf("取り込んだ種目が違う: %+v", row)
	}
	for r, want := range map[training.MuscleRegion]float64{training.TrapMid: 1.0, training.Lat: 0.5, training.Biceps: 0.5} {
		if c, ok := row.Stimulus().Contribution(r); !ok || c.Float() != want {
			t.Errorf("%s の寄与が %v（期待 %v）", r, c.Float(), want)
		}
	}
	if d := byID["u-0000000000000002"]; d == nil || !d.IsDeleted() {
		t.Errorf("消した旧版の種目が消えた状態で取り込まれていない: %+v", d)
	}
	// 足した時刻と消した時刻を引き継ぐ（取り込んだ時刻で上書きしない）。
	var gotCreated time.Time
	var gotDeleted *time.Time
	if err := pool.QueryRow(ctx,
		"SELECT created_at, deleted_at FROM user_exercises WHERE user_id = $1 AND id = $2",
		a.String(), "u-0000000000000002").Scan(&gotCreated, &gotDeleted); err != nil {
		t.Fatalf("取り込んだ行を読めない: %v", err)
	}
	if !gotCreated.Equal(created) || gotDeleted == nil || !gotDeleted.Equal(deletedAt) {
		t.Errorf("時刻が引き継がれていない: created=%v deleted=%v（期待 %v, %v）", gotCreated, gotDeleted, created, deletedAt)
	}

	// 他人の旧版の行は、その人が初めて読んだときにその人の一覧へ入る。
	// A の読み出しで B の行まで取り込むと、B の一覧が0件でなくなり、
	// B にはプリセットが入らないまま自分のマシン1件だけになる。
	gotB, err := repo.FindAll(ctx, b)
	if err != nil {
		t.Fatalf("B の取得に失敗: %v", err)
	}
	if len(gotB) != len(seedAll)+1 {
		t.Errorf("B の件数が %d（期待 %d：プリセット＋B の旧版1件）", len(gotB), len(seedAll)+1)
	}

	// 二度目は入れ直さない。
	again, err := repo.FindAll(ctx, a)
	if err != nil {
		t.Fatalf("二度目の取得に失敗: %v", err)
	}
	if len(again) != len(got) {
		t.Errorf("二度目の件数が %d（期待 %d）", len(again), len(got))
	}
}

// 保存済みの行が1つ読めなくても、その人の種目一覧は読める（#217）。
//
// 行は読み出しのたびにコンストラクタを通す。筋区分の改名や上限の変更で
// 1行でも通らなくなったとき、一覧を丸ごと失敗させると、今日の計画・記録・
// 履歴・設定がその利用者だけ全滅し、ジムで記録がつけられなくなる。
// 読めない行は一覧から外す。行そのものは消さない（直せば戻る）。
func TestExerciseRepository_Postgres_SkipsUnreadableRows(t *testing.T) {
	cases := []struct {
		name     string
		stimulus string
	}{
		// 筋区分を改名・削除したあとの行
		{name: "検証を通らない行", stimulus: `{"no_such_region": 1.0}`},
		// 効き方の列が壊れている行
		{name: "効き方を解釈できない行", stimulus: `[1, 2]`},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			ctx := context.Background()
			seedAll, err := seed.Exercises()
			if err != nil {
				t.Fatalf("シードが不正: %v", err)
			}
			pool := migratedDB(t)
			repo := postgres.NewExerciseRepository(pool, seedAll)
			a := newUser(t)

			if _, err := repo.FindAll(ctx, a); err != nil {
				t.Fatalf("初回の取得に失敗: %v", err)
			}
			if _, err := pool.Exec(ctx, `
				INSERT INTO user_exercises (user_id, id, name, stimulus, increment_kg)
				VALUES ($1, 'u-broken', '壊れた種目', $2::jsonb, 2.5)`, a.String(), c.stimulus); err != nil {
				t.Fatalf("壊れた行を入れられない: %v", err)
			}

			got, err := repo.FindAll(ctx, a)
			if err != nil {
				t.Fatalf("1行が読めないだけで一覧が丸ごと読めない: %v", err)
			}
			if len(got) != len(seedAll) {
				t.Errorf("件数が %d。読めない行だけを外したシードの %d 件のはず", len(got), len(seedAll))
			}
			if findByID(got, "u-broken") != nil {
				t.Error("読めない行が一覧に入っている")
			}

			var left int
			if err := pool.QueryRow(ctx,
				`SELECT count(*) FROM user_exercises WHERE user_id = $1 AND id = 'u-broken'`, a.String(),
			).Scan(&left); err != nil {
				t.Fatal(err)
			}
			if left != 1 {
				t.Error("読めない行が消えている。外すのは一覧からだけで、行は残す")
			}
		})
	}
}
