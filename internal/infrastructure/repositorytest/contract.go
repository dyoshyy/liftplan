// Package repositorytest はリポジトリ実装が満たすべき契約のテストスイート。
//
// 契約（D-033）は「冪等・同一IDで内容が違えば ErrConflictingSetLog・
// 全か無か・取得順の安定・内部状態をエイリアスしない」。これを実装ごとに
// 書き直すと、片方だけが契約を満たす状態に気づけない。同じテストコードを
// インメモリ実装と Postgres 実装の両方に流す。
//
// インメモリ実装は失敗しないので、契約の破れは Postgres で初めて露呈する。
// そのとき「インメモリでは通っていた」では原因の切り分けにならない。
package repositorytest

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/dyoshyy/liftplan-server/internal/domain/training"
)

// day は契約テストで使う基準日。
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

func logIDs(h training.History) []training.SetLogID {
	out := make([]training.SetLogID, 0, len(h.Logs()))
	for _, l := range h.Logs() {
		out = append(out, l.ID())
	}
	return out
}

// RunSetLogContract は SetLogRepository の契約を検証する。
//
// newRepo は毎回空のリポジトリを返すこと。状態が残ると、
// 前のテストの書き込みが次のテストの前提を壊す。
func RunSetLogContract(t *testing.T, newRepo func(*testing.T) training.SetLogRepository) {
	ctx := context.Background()

	t.Run("保存したログが取得できる", func(t *testing.T) {
		repo := newRepo(t)
		if err := repo.Save(ctx, []*training.SetLog{mkSetLog(t, "a", 85)}); err != nil {
			t.Fatalf("保存に失敗: %v", err)
		}
		h, err := repo.FindAll(ctx)
		if err != nil {
			t.Fatalf("取得に失敗: %v", err)
		}
		if len(h.Logs()) != 1 {
			t.Fatalf("件数が誤り: %d", len(h.Logs()))
		}
		got := h.Logs()[0]
		if got.ID() != "a" || got.Weight().Kg() != 85 || got.Reps().Int() != 9 || got.RIR().Int() != 2 {
			t.Errorf("値が往復していない: %+v", got)
		}
		if !got.PerformedOn().Equal(day) {
			t.Errorf("日付が往復していない: %v", got.PerformedOn())
		}
	})

	t.Run("空の保存は何もしない", func(t *testing.T) {
		repo := newRepo(t)
		if err := repo.Save(ctx, nil); err != nil {
			t.Errorf("空の保存でエラーになった: %v", err)
		}
		h, _ := repo.FindAll(ctx)
		if len(h.Logs()) != 0 {
			t.Errorf("空なのに書かれた: %d件", len(h.Logs()))
		}
	})

	// クライアントはオフラインで記録して後から送るので、
	// タイムアウト後の再送は日常的に起きる。
	t.Run("同じ内容の再送を受け入れる", func(t *testing.T) {
		repo := newRepo(t)
		log := mkSetLog(t, "a", 85)
		for i := range 3 {
			if err := repo.Save(ctx, []*training.SetLog{log}); err != nil {
				t.Fatalf("%d回目の再送で失敗: %v", i+1, err)
			}
		}
		h, _ := repo.FindAll(ctx)
		if len(h.Logs()) != 1 {
			t.Errorf("再送で重複した: %d件", len(h.Logs()))
		}
	})

	// 黙って上書きすると、どちらが正しいか分からないまま推定1RMが動く。
	t.Run("同じIDで内容が違えば衝突", func(t *testing.T) {
		repo := newRepo(t)
		if err := repo.Save(ctx, []*training.SetLog{mkSetLog(t, "a", 85)}); err != nil {
			t.Fatalf("保存に失敗: %v", err)
		}
		err := repo.Save(ctx, []*training.SetLog{mkSetLog(t, "a", 90)})
		if !errors.Is(err, training.ErrConflictingSetLog) {
			t.Fatalf("衝突が検出されていない: %v", err)
		}
		h, _ := repo.FindAll(ctx)
		if len(h.Logs()) != 1 || h.Logs()[0].Weight().Kg() != 85 {
			t.Errorf("衝突したのに上書きされた: %v", h.Logs())
		}
	})

	t.Run("同一呼び出し内の衝突も検出する", func(t *testing.T) {
		repo := newRepo(t)
		err := repo.Save(ctx, []*training.SetLog{
			mkSetLog(t, "a", 85), mkSetLog(t, "a", 90),
		})
		if !errors.Is(err, training.ErrConflictingSetLog) {
			t.Fatalf("同一呼び出し内の衝突が検出されていない: %v", err)
		}
		h, _ := repo.FindAll(ctx)
		if len(h.Logs()) != 0 {
			t.Errorf("全か無かで書いていない: %d件", len(h.Logs()))
		}
	})

	// 半分だけ保存された状態は、その週の刺激量を実態とずらしたまま
	// 計画に効き続ける。
	t.Run("衝突したら1件も書かない", func(t *testing.T) {
		repo := newRepo(t)
		if err := repo.Save(ctx, []*training.SetLog{mkSetLog(t, "a", 85)}); err != nil {
			t.Fatalf("保存に失敗: %v", err)
		}
		err := repo.Save(ctx, []*training.SetLog{
			mkSetLog(t, "new", 100),
			mkSetLog(t, "a", 90),
		})
		if !errors.Is(err, training.ErrConflictingSetLog) {
			t.Fatalf("衝突が検出されていない: %v", err)
		}
		h, _ := repo.FindAll(ctx)
		if len(h.Logs()) != 1 {
			t.Errorf("衝突したのに一部が書かれた: %d件", len(h.Logs()))
		}
	})

	t.Run("nil を拒否する", func(t *testing.T) {
		repo := newRepo(t)
		if err := repo.Save(ctx, []*training.SetLog{mkSetLog(t, "a", 85), nil}); err == nil {
			t.Error("nil が弾かれていない")
		}
		h, _ := repo.FindAll(ctx)
		if len(h.Logs()) != 0 {
			t.Errorf("nil を含む保存で一部が書かれた: %d件", len(h.Logs()))
		}
	})

	// 順序が揺れると、History の重複解決や推定1RMの畳み込みが
	// 呼び出しごとに変わり、同じ入力から違う計画が出る。
	t.Run("取得順が安定している", func(t *testing.T) {
		repo := newRepo(t)
		logs := make([]*training.SetLog, 0, 32)
		for i := range 32 {
			logs = append(logs, mkSetLog(t, fmt.Sprintf("o%03d", i), 85))
		}
		if err := repo.Save(ctx, logs); err != nil {
			t.Fatalf("保存に失敗: %v", err)
		}

		first, _ := repo.FindAll(ctx)
		want := logIDs(first)
		for n := range 10 {
			got, _ := repo.FindAll(ctx)
			gotIDs := logIDs(got)
			if len(gotIDs) != len(want) {
				t.Fatalf("%d回目で件数が変わった: %d", n+1, len(gotIDs))
			}
			for i := range gotIDs {
				if gotIDs[i] != want[i] {
					t.Fatalf("%d回目で順序が変わった: %d番目が %s（期待 %s）",
						n+1, i, gotIDs[i], want[i])
				}
			}
		}
	})

	t.Run("並行に呼んでも壊れない", func(t *testing.T) {
		repo := newRepo(t)
		var wg sync.WaitGroup
		errs := make([]error, 16)
		for i := range errs {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				errs[i] = repo.Save(ctx, []*training.SetLog{
					mkSetLog(t, fmt.Sprintf("c%03d", i), 85),
				})
				_, _ = repo.FindAll(ctx)
			}(i)
		}
		wg.Wait()

		for i, err := range errs {
			if err != nil {
				t.Errorf("%d番目の保存が失敗: %v", i, err)
			}
		}
		h, _ := repo.FindAll(ctx)
		if len(h.Logs()) != 16 {
			t.Errorf("並行保存で件数が合わない: %d", len(h.Logs()))
		}
	})
}

// RunConditionContract は ConditionRepository の契約を検証する。
func RunConditionContract(t *testing.T, newRepo func(*testing.T) training.ConditionRepository) {
	ctx := context.Background()

	t.Run("保存した値が往復する", func(t *testing.T) {
		repo := newRepo(t)
		item := training.NewDailyCondition(day).WithBodyWeight(75.5).WithSleepHours(7.25)
		if err := repo.Save(ctx, []training.DailyCondition{item}); err != nil {
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
		if kg, ok := got.BodyWeightKg(); !ok || kg != 75.5 {
			t.Errorf("体重が往復していない: %v (%v)", kg, ok)
		}
		if h, ok := got.SleepHours(); !ok || h != 7.25 {
			t.Errorf("睡眠時間が往復していない: %v (%v)", h, ok)
		}
	})

	t.Run("欠損は欠損のまま往復する", func(t *testing.T) {
		repo := newRepo(t)
		if err := repo.Save(ctx, []training.DailyCondition{
			training.NewDailyCondition(day).WithBodyWeight(75),
		}); err != nil {
			t.Fatalf("保存に失敗: %v", err)
		}

		log, _ := repo.FindAll(ctx)
		got, ok := log.On(day)
		if !ok {
			t.Fatal("その日の記録が無い")
		}
		if _, ok := got.SleepHours(); ok {
			t.Error("送っていない睡眠時間に値が付いている")
		}
	})

	// 日付ごと置き換えると、体重だけを送ったときに睡眠時間が消える。
	// クライアントは体重と睡眠を別のタイミングで記録するので日常的に起きる。
	t.Run("同じ日付を項目ごとに合成する", func(t *testing.T) {
		repo := newRepo(t)
		if err := repo.Save(ctx, []training.DailyCondition{
			training.NewDailyCondition(day).WithBodyWeight(75).WithSleepHours(7),
		}); err != nil {
			t.Fatalf("保存に失敗: %v", err)
		}
		if err := repo.Save(ctx, []training.DailyCondition{
			training.NewDailyCondition(day).WithBodyWeight(74),
		}); err != nil {
			t.Fatalf("保存に失敗: %v", err)
		}

		log, _ := repo.FindAll(ctx)
		got, _ := log.On(day)
		if kg, ok := got.BodyWeightKg(); !ok || kg != 74 {
			t.Errorf("体重が更新されていない: %v (%v)", kg, ok)
		}
		if h, ok := got.SleepHours(); !ok || h != 7 {
			t.Errorf("睡眠時間が消えた: %v (%v)", h, ok)
		}
	})

	t.Run("同一呼び出し内でも合成する", func(t *testing.T) {
		repo := newRepo(t)
		if err := repo.Save(ctx, []training.DailyCondition{
			training.NewDailyCondition(day).WithSleepHours(6),
			training.NewDailyCondition(day).WithBodyWeight(73),
		}); err != nil {
			t.Fatalf("保存に失敗: %v", err)
		}

		log, _ := repo.FindAll(ctx)
		got, _ := log.On(day)
		if h, ok := got.SleepHours(); !ok || h != 6 {
			t.Errorf("睡眠時間が失われた: %v (%v)", h, ok)
		}
		if kg, ok := got.BodyWeightKg(); !ok || kg != 73 {
			t.Errorf("体重が失われた: %v (%v)", kg, ok)
		}
		if log.Len() != 1 {
			t.Errorf("同じ日付が重複した: %d件", log.Len())
		}
	})

	// 捨てると、クライアントは保存に成功したと思ったまま記録が消える。
	t.Run("日付の無い記録を拒否する", func(t *testing.T) {
		repo := newRepo(t)
		err := repo.Save(ctx, []training.DailyCondition{
			training.NewDailyCondition(day).WithBodyWeight(75),
			{},
		})
		if err == nil {
			t.Error("日付の無い記録が弾かれていない")
		}
		log, _ := repo.FindAll(ctx)
		if log.Len() != 0 {
			t.Errorf("全か無かで書いていない: %d件", log.Len())
		}
	})

	t.Run("取得順が日付の昇順である", func(t *testing.T) {
		repo := newRepo(t)
		items := make([]training.DailyCondition, 0, 10)
		for i := range 10 {
			items = append(items,
				training.NewDailyCondition(day.AddDays(-i)).WithBodyWeight(70+float64(i)))
		}
		if err := repo.Save(ctx, items); err != nil {
			t.Fatalf("保存に失敗: %v", err)
		}

		log, _ := repo.FindAll(ctx)
		got := log.Items()
		if len(got) != 10 {
			t.Fatalf("件数が誤り: %d", len(got))
		}
		for i := 1; i < len(got); i++ {
			if !got[i-1].Date().Before(got[i].Date()) {
				t.Fatalf("日付の昇順でない: %v の後に %v",
					got[i-1].Date(), got[i].Date())
			}
		}
	})

	t.Run("並行に呼んでも壊れない", func(t *testing.T) {
		repo := newRepo(t)
		var wg sync.WaitGroup
		errs := make([]error, 16)
		for i := range errs {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				errs[i] = repo.Save(ctx, []training.DailyCondition{
					training.NewDailyCondition(day.AddDays(-i)).WithBodyWeight(75),
				})
				_, _ = repo.FindAll(ctx)
			}(i)
		}
		wg.Wait()

		for i, err := range errs {
			if err != nil {
				t.Errorf("%d番目の保存が失敗: %v", i, err)
			}
		}
		log, _ := repo.FindAll(ctx)
		if log.Len() != 16 {
			t.Errorf("並行保存で件数が合わない: %d", log.Len())
		}
	})
}

// RunProgramContract は ProgramRepository の契約を検証する。
func RunProgramContract(t *testing.T, newRepo func(*testing.T) training.ProgramRepository) {
	ctx := context.Background()

	program := func(t *testing.T, perWeek int) *training.Program {
		t.Helper()
		freq, err := training.NewFrequency(perWeek)
		if err != nil {
			t.Fatalf("頻度が不正: %v", err)
		}
		target, err := training.NewWeeklyVolumeTarget(map[training.MuscleRegion]float64{
			training.ChestMid: 14, training.Quad: 16,
		})
		if err != nil {
			t.Fatalf("週目標が不正: %v", err)
		}
		p, err := training.NewProgram(freq, target,
			[]training.ExerciseID{"bench", "squat", "deadlift"})
		if err != nil {
			t.Fatalf("プログラムが不正: %v", err)
		}
		return p
	}

	// (nil, nil) を返されると、呼び出し側が nil を「未設定」と
	// 「取得成功」のどちらとも解釈できてしまう。
	t.Run("未設定はセンチネルで返る", func(t *testing.T) {
		repo := newRepo(t)
		p, err := repo.Get(ctx)
		if !errors.Is(err, training.ErrProgramNotConfigured) {
			t.Errorf("未設定のセンチネルが返らない: %v", err)
		}
		if p != nil {
			t.Error("未設定なのにプログラムが返った")
		}
	})

	t.Run("保存した値が往復する", func(t *testing.T) {
		repo := newRepo(t)
		if err := repo.Save(ctx, program(t, 3)); err != nil {
			t.Fatalf("保存に失敗: %v", err)
		}

		got, err := repo.Get(ctx)
		if err != nil {
			t.Fatalf("取得に失敗: %v", err)
		}
		if got.Frequency().PerWeek() != 3 {
			t.Errorf("頻度が往復していない: %d", got.Frequency().PerWeek())
		}
		if got.WeeklyTarget().Sets(training.ChestMid) != 14 {
			t.Errorf("週目標が往復していない: %v", got.WeeklyTarget().Sets(training.ChestMid))
		}
		if len(got.SelectedExercises()) != 3 || !got.Includes("bench") {
			t.Errorf("選択種目が往復していない: %v", got.SelectedExercises())
		}
	})

	// プログラムはユーザーごとに1つ。保存は常に全体の置き換え。
	t.Run("保存は冪等で置き換えになる", func(t *testing.T) {
		repo := newRepo(t)
		if err := repo.Save(ctx, program(t, 3)); err != nil {
			t.Fatalf("保存に失敗: %v", err)
		}
		if err := repo.Save(ctx, program(t, 4)); err != nil {
			t.Fatalf("2度目の保存に失敗: %v", err)
		}

		got, err := repo.Get(ctx)
		if err != nil {
			t.Fatalf("取得に失敗: %v", err)
		}
		if got.Frequency().PerWeek() != 4 {
			t.Errorf("置き換わっていない: %d", got.Frequency().PerWeek())
		}
	})

	t.Run("nil を拒否する", func(t *testing.T) {
		repo := newRepo(t)
		if err := repo.Save(ctx, nil); err == nil {
			t.Error("nil のプログラムが保存された")
		}
		if _, err := repo.Get(ctx); !errors.Is(err, training.ErrProgramNotConfigured) {
			t.Errorf("未設定のままでない: %v", err)
		}
	})

	t.Run("並行に呼んでも壊れない", func(t *testing.T) {
		repo := newRepo(t)
		if err := repo.Save(ctx, program(t, 3)); err != nil {
			t.Fatalf("保存に失敗: %v", err)
		}

		var wg sync.WaitGroup
		errs := make([]error, 16)
		for i := range errs {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				if i%2 == 0 {
					errs[i] = repo.Save(ctx, program(t, 3))
					return
				}
				_, errs[i] = repo.Get(ctx)
			}(i)
		}
		wg.Wait()

		for i, err := range errs {
			if err != nil {
				t.Errorf("%d番目が失敗: %v", i, err)
			}
		}
	})
}
