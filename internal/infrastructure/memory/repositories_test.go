package memory_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/dyoshyy/liftplan-server/internal/domain/training"
	"github.com/dyoshyy/liftplan-server/internal/domain/training/seed"
	"github.com/dyoshyy/liftplan-server/internal/infrastructure/memory"
)

var day = training.MustDate(2026, time.August, 17)

func mkSetLog(t *testing.T, id string, kg float64) *training.SetLog {
	t.Helper()
	l, err := training.NewSetLog(training.SetLogParams{
		ID: id, PerformedOn: day, ExerciseID: "bench",
		WeightKg: kg, Reps: 9, RIR: 2,
	})
	if err != nil {
		t.Fatalf("ログ生成に失敗: %v", err)
	}
	return l
}

func TestExerciseRepository_ReturnsSeed(t *testing.T) {
	all, err := seed.Exercises()
	if err != nil {
		t.Fatalf("シードが不正: %v", err)
	}
	repo := memory.NewExerciseRepository(all)

	got, err := repo.FindAll(context.Background())
	if err != nil {
		t.Fatalf("取得に失敗: %v", err)
	}
	if len(got) != len(all) {
		t.Errorf("件数が誤り: got %d, want %d", len(got), len(all))
	}
}

func TestSetLogRepository_SaveIsIdempotent(t *testing.T) {
	repo := memory.NewSetLogRepository()
	ctx := context.Background()

	log := mkSetLog(t, "01J-SAME", 85)
	if err := repo.Save(ctx, []*training.SetLog{log}); err != nil {
		t.Fatalf("1回目の保存に失敗: %v", err)
	}
	if err := repo.Save(ctx, []*training.SetLog{log}); err != nil {
		t.Fatalf("2回目の保存に失敗: %v", err)
	}

	history, err := repo.FindAll(ctx)
	if err != nil {
		t.Fatalf("取得に失敗: %v", err)
	}
	if got := len(history.Logs()); got != 1 {
		t.Errorf("同じIDが重複して保存された: %d件", got)
	}
}

// 同じIDで内容が違うログは黙って上書きしない。
// 上書きすると、どちらが正しいか分からないまま推定1RMが動く。
func TestSetLogRepository_RejectsConflictingSameID(t *testing.T) {
	repo := memory.NewSetLogRepository()
	ctx := context.Background()

	if err := repo.Save(ctx, []*training.SetLog{mkSetLog(t, "01J-X", 85)}); err != nil {
		t.Fatalf("保存に失敗: %v", err)
	}
	err := repo.Save(ctx, []*training.SetLog{mkSetLog(t, "01J-X", 90)})
	if !errors.Is(err, training.ErrConflictingSetLog) {
		t.Errorf("衝突が検出されていない: %v", err)
	}

	history, _ := repo.FindAll(ctx)
	logs := history.Logs()
	if len(logs) != 1 {
		t.Fatalf("件数が誤り: %d", len(logs))
	}
	if logs[0].Weight().Kg() != 85 {
		t.Errorf("衝突したのに上書きされた: %v", logs[0].Weight().Kg())
	}
}

// 同じ呼び出しの中に衝突があっても検出すること。
func TestSetLogRepository_RejectsConflictWithinOneCall(t *testing.T) {
	repo := memory.NewSetLogRepository()
	err := repo.Save(context.Background(), []*training.SetLog{
		mkSetLog(t, "01J-Y", 85),
		mkSetLog(t, "01J-Y", 90),
	})
	if !errors.Is(err, training.ErrConflictingSetLog) {
		t.Errorf("同一呼び出し内の衝突が検出されていない: %v", err)
	}
	if repo.Size() != 0 {
		t.Errorf("全か無かで書いていない: %d件", repo.Size())
	}
}

// 衝突を見つけたら1件も書かない。半分だけ保存された状態は、
// その週の刺激量を実態とずらしたまま計画に効き続ける。
func TestSetLogRepository_SaveIsAllOrNothing(t *testing.T) {
	repo := memory.NewSetLogRepository()
	ctx := context.Background()

	if err := repo.Save(ctx, []*training.SetLog{mkSetLog(t, "01J-Z", 85)}); err != nil {
		t.Fatalf("保存に失敗: %v", err)
	}
	err := repo.Save(ctx, []*training.SetLog{
		mkSetLog(t, "01J-NEW", 100),
		mkSetLog(t, "01J-Z", 90), // 既存と衝突
	})
	if !errors.Is(err, training.ErrConflictingSetLog) {
		t.Fatalf("衝突が検出されていない: %v", err)
	}
	if repo.Size() != 1 {
		t.Errorf("衝突したのに一部が書かれた: %d件", repo.Size())
	}
}

// 同じ内容の再送は黙って受け入れること。
// クライアントはタイムアウト後に再送するので、これは日常的に起きる。
func TestSetLogRepository_AcceptsIdenticalResend(t *testing.T) {
	repo := memory.NewSetLogRepository()
	ctx := context.Background()

	log := mkSetLog(t, "01J-R", 85)
	for range 3 {
		if err := repo.Save(ctx, []*training.SetLog{log}); err != nil {
			t.Fatalf("同じ内容の再送で失敗: %v", err)
		}
	}
	if repo.Size() != 1 {
		t.Errorf("件数が誤り: %d", repo.Size())
	}
}

func TestSetLogRepository_RejectsNilEntries(t *testing.T) {
	repo := memory.NewSetLogRepository()
	err := repo.Save(context.Background(), []*training.SetLog{
		mkSetLog(t, "01J-N", 85), nil,
	})
	if err == nil {
		t.Error("nil が弾かれていない")
	}
	if repo.Size() != 0 {
		t.Errorf("nil を含む保存で一部が書かれた: %d件", repo.Size())
	}
}

func TestConditionRepository_SaveOverwritesSameDate(t *testing.T) {
	repo := memory.NewConditionRepository()
	ctx := context.Background()

	_ = repo.Save(ctx, []training.DailyCondition{
		training.NewDailyCondition(day).WithBodyWeight(75),
	})
	_ = repo.Save(ctx, []training.DailyCondition{
		training.NewDailyCondition(day).WithBodyWeight(74),
	})

	log, err := repo.FindAll(ctx)
	if err != nil {
		t.Fatalf("取得に失敗: %v", err)
	}
	trend, ok := training.DefaultConditionAnalyzer().BodyWeightTrendKgPerWeek(log, day)
	_ = trend
	if ok {
		t.Log("サンプル数が足りればトレンドが出る（ここでは1件なので false が正しい）")
	}
	// 件数の確認は ConditionLog に件数APIが無いため、
	// 同日2回保存で 1 件になっていることを Save の戻り値ではなく Size で確かめる
	if got := repo.Size(); got != 1 {
		t.Errorf("同じ日付が重複して保存された: %d件", got)
	}
}

func TestProgramRepository_NotConfigured(t *testing.T) {
	repo := memory.NewProgramRepository(nil)
	if _, err := repo.Get(context.Background()); err != training.ErrProgramNotConfigured {
		t.Errorf("未設定エラーが返らない: %v", err)
	}
}

func TestProgramRepository_Save(t *testing.T) {
	pool, _ := seed.Exercises()
	freq, _ := training.NewFrequency(3)
	target, _ := seed.DefaultWeeklyTarget(freq)
	selected := make([]training.ExerciseID, 0, len(pool))
	for _, e := range pool {
		if e.Kind() != training.KindVariation {
			selected = append(selected, e.ID())
		}
	}
	program, err := training.NewProgram(freq, target, selected)
	if err != nil {
		t.Fatalf("プログラムが不正: %v", err)
	}

	repo := memory.NewProgramRepository(nil)
	if err := repo.Save(context.Background(), program); err != nil {
		t.Fatalf("保存に失敗: %v", err)
	}

	got, err := repo.Get(context.Background())
	if err != nil {
		t.Fatalf("取得に失敗: %v", err)
	}
	if got.Frequency().PerWeek() != 3 {
		t.Errorf("頻度が誤り: %d", got.Frequency().PerWeek())
	}
}

func TestSetLogRepository_ConcurrentSaveIsSafe(t *testing.T) {
	repo := memory.NewSetLogRepository()
	ctx := context.Background()

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			id := "01J-" + string(rune('A'+n%26)) + string(rune('a'+n/26))
			_ = repo.Save(ctx, []*training.SetLog{mkSetLog(t, id, 85)})
		}(i)
	}
	wg.Wait()

	if _, err := repo.FindAll(ctx); err != nil {
		t.Fatalf("並行保存後の取得に失敗: %v", err)
	}
}

// 同じ日付は項目ごとに上書きすること。日付ごと置き換えると、
// 体重だけを送ったときに睡眠時間が消える。クライアントは体重と睡眠を
// 別のタイミングで記録するので、これは日常的に起きる。
func TestConditionRepository_MergesFieldWise(t *testing.T) {
	repo := memory.NewConditionRepository()
	ctx := context.Background()

	if err := repo.Save(ctx, []training.DailyCondition{
		training.NewDailyCondition(day).WithBodyWeight(75).WithSleepHours(7),
	}); err != nil {
		t.Fatalf("保存に失敗: %v", err)
	}
	// 体重だけを送り直す。
	if err := repo.Save(ctx, []training.DailyCondition{
		training.NewDailyCondition(day).WithBodyWeight(74),
	}); err != nil {
		t.Fatalf("保存に失敗: %v", err)
	}

	log, err := repo.FindAll(ctx)
	if err != nil {
		t.Fatalf("取得に失敗: %v", err)
	}
	got, ok := log.On(day)
	if !ok {
		t.Fatal("その日の記録が無い")
	}
	if kg, ok := got.BodyWeightKg(); !ok || kg != 74 {
		t.Errorf("体重が更新されていない: %v (%v)", kg, ok)
	}
	if h, ok := got.SleepHours(); !ok || h != 7 {
		t.Errorf("睡眠時間が消えた: %v (%v)", h, ok)
	}
}

// 同じ呼び出しの中に同じ日付が複数あっても、項目ごとに合成すること。
func TestConditionRepository_MergesWithinOneCall(t *testing.T) {
	repo := memory.NewConditionRepository()
	if err := repo.Save(context.Background(), []training.DailyCondition{
		training.NewDailyCondition(day).WithSleepHours(6),
		training.NewDailyCondition(day).WithBodyWeight(73),
	}); err != nil {
		t.Fatalf("保存に失敗: %v", err)
	}

	log, _ := repo.FindAll(context.Background())
	got, ok := log.On(day)
	if !ok {
		t.Fatal("その日の記録が無い")
	}
	if h, ok := got.SleepHours(); !ok || h != 6 {
		t.Errorf("睡眠時間が失われた: %v (%v)", h, ok)
	}
	if kg, ok := got.BodyWeightKg(); !ok || kg != 73 {
		t.Errorf("体重が失われた: %v (%v)", kg, ok)
	}
	if repo.Size() != 1 {
		t.Errorf("同じ日付が重複した: %d件", repo.Size())
	}
}

// FindAll が返す値がリポジトリ内部の状態をエイリアスしないこと。
func TestExerciseRepository_DoesNotAliasItsState(t *testing.T) {
	pool, err := seed.Exercises()
	if err != nil {
		t.Fatalf("シードが不正: %v", err)
	}
	repo := memory.NewExerciseRepository(pool)

	// 呼び出し側がスライスを壊しても、次の取得に影響しない。
	got, _ := repo.FindAll(context.Background())
	for i := range got {
		got[i] = nil
	}

	again, _ := repo.FindAll(context.Background())
	for i, e := range again {
		if e == nil {
			t.Fatalf("%d番目が nil になっている", i)
		}
	}

	// コンストラクタに渡したスライスを後から壊しても影響しない。
	pool[0] = nil
	third, _ := repo.FindAll(context.Background())
	if third[0] == nil {
		t.Error("コンストラクタの引数をエイリアスしている")
	}
}

// Save と FindAll を並行に呼んでも壊れないこと。
func TestRepositories_AreSafeForConcurrentUse(t *testing.T) {
	logs := memory.NewSetLogRepository()
	conditions := memory.NewConditionRepository()
	programs := memory.NewProgramRepository(nil)
	ctx := context.Background()

	var wg sync.WaitGroup
	for i := range 16 {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_ = logs.Save(ctx, []*training.SetLog{mkSetLog(t, fmt.Sprintf("c%03d", i), 85)})
			_, _ = logs.FindAll(ctx)
			_ = conditions.Save(ctx, []training.DailyCondition{
				training.NewDailyCondition(day.AddDays(-i)).WithBodyWeight(75),
			})
			_, _ = conditions.FindAll(ctx)
			_, _ = programs.Get(ctx)
		}(i)
	}
	wg.Wait()

	if logs.Size() != 16 {
		t.Errorf("並行保存で件数が合わない: %d", logs.Size())
	}
	if conditions.Size() != 16 {
		t.Errorf("並行保存で件数が合わない: %d", conditions.Size())
	}
}

// 取得順が呼び出しごとに変わらないこと。
//
// 順序が揺れると、History の重複解決（後にあるものを採用）や
// 推定1RMの畳み込みの結果が呼び出しごとに変わり、同じ入力から
// 違う計画が出る。map の range は順序を保証しない。
func TestSetLogRepository_ReturnsAStableOrder(t *testing.T) {
	repo := memory.NewSetLogRepository()
	ctx := context.Background()

	logs := make([]*training.SetLog, 0, 32)
	for i := range 32 {
		logs = append(logs, mkSetLog(t, fmt.Sprintf("o%03d", i), 85))
	}
	if err := repo.Save(ctx, logs); err != nil {
		t.Fatalf("保存に失敗: %v", err)
	}

	first, _ := repo.FindAll(ctx)
	want := make([]training.SetLogID, 0, 32)
	for _, l := range first.Logs() {
		want = append(want, l.ID())
	}

	for n := range 20 {
		got, _ := repo.FindAll(ctx)
		for i, l := range got.Logs() {
			if l.ID() != want[i] {
				t.Fatalf("%d回目の取得で順序が変わった: %d番目が %s（期待 %s）",
					n+1, i, l.ID(), want[i])
			}
		}
	}
}

func TestProgramRepository_RejectsNil(t *testing.T) {
	repo := memory.NewProgramRepository(nil)
	if err := repo.Save(context.Background(), nil); err == nil {
		t.Error("nil のプログラムが保存された")
	}
	if _, err := repo.Get(context.Background()); !errors.Is(err, training.ErrProgramNotConfigured) {
		t.Errorf("未設定のままでない: %v", err)
	}
}
