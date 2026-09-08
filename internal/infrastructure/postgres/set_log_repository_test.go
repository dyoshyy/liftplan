package postgres_test

import (
	"context"
	"testing"

	"github.com/dyoshyy/liftplan-server/internal/domain/training"
	"github.com/dyoshyy/liftplan-server/internal/infrastructure/postgres"

	"github.com/dyoshyy/liftplan-server/internal/domain/training/setlog"
)

// 再起動しても記録が残ること。インメモリ実装との唯一の違いがここ。
func TestSetLogRepository_SurvivesReconnect(t *testing.T) {
	pool := migratedDB(t)
	ctx := context.Background()

	log, err := setlog.NewSetLog(setlog.SetLogParams{
		ID: "persist", PerformedOn: training.MustDate(2026, 8, 17),
		ExerciseID: "bench", WeightKg: 87.5, Reps: 8, RIR: 2,
	})
	if err != nil {
		t.Fatalf("ログ生成に失敗: %v", err)
	}
	if err := postgres.NewSetLogRepository(pool).Save(ctx, []*setlog.SetLog{log}); err != nil {
		t.Fatalf("保存に失敗: %v", err)
	}

	// 別のリポジトリインスタンスから読む。
	h, err := postgres.NewSetLogRepository(pool).FindAll(ctx)
	if err != nil {
		t.Fatalf("取得に失敗: %v", err)
	}
	if len(h.Logs()) != 1 {
		t.Fatalf("件数が誤り: %d", len(h.Logs()))
	}
	if got := h.Logs()[0].Weight().Kg(); got != 87.5 {
		t.Errorf("小数が往復していない: %v（期待 87.5）", got)
	}
}

// 2.5kg 刻みの重量が往復すること。
// double precision だと 87.5 のような値が往復で揺れうる。
func TestSetLogRepository_RoundTripsWeights(t *testing.T) {
	repo := postgres.NewSetLogRepository(migratedDB(t))
	ctx := context.Background()

	want := []float64{0, 1.25, 2.5, 42.5, 87.5, 102.5, 187.5, 999.99}
	logs := make([]*setlog.SetLog, 0, len(want))
	for i, kg := range want {
		l, err := setlog.NewSetLog(setlog.SetLogParams{
			ID: string(rune('a' + i)), PerformedOn: training.MustDate(2026, 8, 17),
			ExerciseID: "bench", WeightKg: kg, Reps: 8, RIR: 2,
		})
		if err != nil {
			t.Fatalf("%vkg のログ生成に失敗: %v", kg, err)
		}
		logs = append(logs, l)
	}
	if err := repo.Save(ctx, logs); err != nil {
		t.Fatalf("保存に失敗: %v", err)
	}

	h, _ := repo.FindAll(ctx)
	got := make(map[setlog.SetLogID]float64, len(h.Logs()))
	for _, l := range h.Logs() {
		got[l.ID()] = l.Weight().Kg()
	}
	for i, kg := range want {
		id := setlog.SetLogID(rune('a' + i))
		if got[id] != kg {
			t.Errorf("%vkg が %v になった", kg, got[id])
		}
	}
}

// 日付がタイムゾーンでずれないこと。
// date 型に時刻もタイムゾーンも無いので、ローカルタイムで渡すと
// サーバーの設定次第で1日ずれる。
func TestSetLogRepository_RoundTripsDates(t *testing.T) {
	repo := postgres.NewSetLogRepository(migratedDB(t))
	ctx := context.Background()

	dates := []training.Date{
		training.MustDate(2026, 1, 1),
		training.MustDate(2026, 8, 17),
		training.MustDate(2026, 12, 31),
		training.MustDate(2024, 2, 29), // 閏日
	}
	logs := make([]*setlog.SetLog, 0, len(dates))
	for i, d := range dates {
		l, err := setlog.NewSetLog(setlog.SetLogParams{
			ID: string(rune('a' + i)), PerformedOn: d,
			ExerciseID: "bench", WeightKg: 85, Reps: 8, RIR: 2,
		})
		if err != nil {
			t.Fatalf("ログ生成に失敗: %v", err)
		}
		logs = append(logs, l)
	}
	if err := repo.Save(ctx, logs); err != nil {
		t.Fatalf("保存に失敗: %v", err)
	}

	h, _ := repo.FindAll(ctx)
	got := make(map[setlog.SetLogID]training.Date, len(h.Logs()))
	for _, l := range h.Logs() {
		got[l.ID()] = l.PerformedOn()
	}
	for i, d := range dates {
		id := setlog.SetLogID(rune('a' + i))
		if !got[id].Equal(d) {
			t.Errorf("%v が %v になった", d, got[id])
		}
	}
}

// 並行に同じIDを書いても、衝突を見逃さないこと。
// 比較と INSERT の間に別のトランザクションが割り込む窓があってはいけない。
func TestSetLogRepository_DetectsConflictUnderConcurrency(t *testing.T) {
	repo := postgres.NewSetLogRepository(migratedDB(t))
	ctx := context.Background()

	mk := func(kg float64) *setlog.SetLog {
		l, err := setlog.NewSetLog(setlog.SetLogParams{
			ID: "race", PerformedOn: training.MustDate(2026, 8, 17),
			ExerciseID: "bench", WeightKg: kg, Reps: 8, RIR: 2,
		})
		if err != nil {
			t.Fatalf("ログ生成に失敗: %v", err)
		}
		return l
	}

	results := make(chan error, 8)
	for i := range 8 {
		go func(i int) {
			results <- repo.Save(ctx, []*setlog.SetLog{mk(80 + float64(i)*2.5)})
		}(i)
	}

	succeeded := 0
	for range 8 {
		if err := <-results; err == nil {
			succeeded++
		}
	}
	// 内容が全部違うので、成功するのは最初の1つだけ。
	if succeeded != 1 {
		t.Errorf("内容の違う並行書き込みが %d 件成功した（期待 1）", succeeded)
	}

	h, _ := repo.FindAll(ctx)
	if len(h.Logs()) != 1 {
		t.Errorf("件数が誤り: %d", len(h.Logs()))
	}
}

// DB に不正な値が入っていたら、境界で止めてドメインに届けないこと。
// 通すと、ありえない RIR や負のレップが推定1RMに効く。
func TestSetLogRepository_RejectsInvalidStoredRows(t *testing.T) {
	pool := migratedDB(t)
	ctx := context.Background()

	if _, err := pool.Exec(ctx, `
		INSERT INTO set_logs (id, performed_on, exercise_id, weight_kg, reps, rir)
		VALUES ('broken', DATE '2026-08-17', 'bench', 85, -5, 2)`); err != nil {
		t.Fatalf("準備に失敗: %v", err)
	}

	if _, err := postgres.NewSetLogRepository(pool).FindAll(ctx); err == nil {
		t.Error("不正な行が黙って読み込まれた")
	}
}

// 保存した日付が DB 上でも同じ日であること。
//
// date 型に時刻もタイムゾーンも無いので、ローカルタイムで渡すと
// サーバーのタイムゾーン設定で1日ずれる。ドメインを経由して読み直すと
// 同じずれで戻ってきて気づけないため、生の値を見る。
func TestSetLogRepository_StoresTheSameCalendarDate(t *testing.T) {
	pool := migratedDB(t)
	ctx := context.Background()

	log, err := setlog.NewSetLog(setlog.SetLogParams{
		ID: "tz", PerformedOn: training.MustDate(2026, 8, 17),
		ExerciseID: "bench", WeightKg: 85, Reps: 8, RIR: 2,
	})
	if err != nil {
		t.Fatalf("ログ生成に失敗: %v", err)
	}
	if err := postgres.NewSetLogRepository(pool).Save(ctx, []*setlog.SetLog{log}); err != nil {
		t.Fatalf("保存に失敗: %v", err)
	}

	var stored string
	if err := pool.QueryRow(ctx,
		"SELECT performed_on::text FROM set_logs WHERE id = 'tz'").Scan(&stored); err != nil {
		t.Fatalf("生の値を読めない: %v", err)
	}
	if stored != "2026-08-17" {
		t.Errorf("DB 上の日付がずれている: %s", stored)
	}
}
