package postgres_test

import (
	"context"
	"encoding/json"
	"maps"
	"os"
	"testing"

	"github.com/dyoshyy/liftplan/internal/domain/training/seed"
	"github.com/dyoshyy/liftplan/internal/infrastructure/postgres"
)

// RDL とデッドリフトの効き方が、シードの旧値のままの行だけシードの新しい値に
// 置き換わる（0022）。本人が直した行と、対象でない種目は触らない。
//
// 期待値をシードから引くのは、シードとマイグレーションの値がずれると
// 新規の利用者と既存の利用者で効き方が食い違うため。
//
// 0017 と同じく、全部のマイグレーションを当てたあとで行を入れ、0022 の SQL を
// もう一度流す（条件付きの UPDATE だけなので何度流しても同じ）。
func TestMigration0022_AddsTrapToHinges(t *testing.T) {
	pool := newTestDB(t)
	ctx := context.Background()
	if err := postgres.Migrate(ctx, pool); err != nil {
		t.Fatalf("マイグレーションに失敗: %v", err)
	}
	sql, err := os.ReadFile("migrations/0022_add_trap_to_hinges.sql")
	if err != nil {
		t.Fatalf("0022 を読めない: %v", err)
	}

	const (
		oldRDL      = `{"HAMSTRING": 1, "GLUTE": 0.7, "ERECTOR": 0.7}`
		oldDeadlift = `{"HAMSTRING": 1.0, "GLUTE": 0.8, "ERECTOR": 1.0, "TRAP_MID": 0.4, "FOREARM": 0.4, "QUAD": 0.3, "LAT": 0.3}`
		edited      = `{"HAMSTRING": 1.0, "GLUTE": 0.5}`
	)
	users := struct{ a, b, c string }{
		"11111111-1111-1111-1111-111111111111",
		"22222222-2222-2222-2222-222222222222",
		"33333333-3333-3333-3333-333333333333",
	}
	rows := []struct {
		user, id, stimulus string
		deleted            bool
	}{
		// シードの旧値のまま → シードの新しい値になる。
		{users.a, "romanian_deadlift", oldRDL, false},
		{users.a, "deadlift", oldDeadlift, false},
		// 消した行も置き換える。戻したときに旧値が蘇らないように。
		{users.b, "romanian_deadlift", oldRDL, true},
		// 本人が直した行（旧値と違う）→ 触らない。
		{users.c, "romanian_deadlift", edited, false},
		// 対象でない種目は、同じ値でも触らない。
		{users.a, "good_morning", oldRDL, false},
	}
	for _, r := range rows {
		deleted := any(nil)
		if r.deleted {
			deleted = "2026-09-01"
		}
		if _, err := pool.Exec(ctx, `
			INSERT INTO user_exercises (user_id, id, name, stimulus, increment_kg, deleted_at)
			VALUES ($1, $2, $2, $3::jsonb, 2.5, $4::timestamptz)`,
			r.user, r.id, r.stimulus, deleted); err != nil {
			t.Fatalf("%s を入れられない: %v", r.id, err)
		}
	}

	if _, err := pool.Exec(ctx, string(sql)); err != nil {
		t.Fatalf("0022 が失敗: %v", err)
	}

	seeded := seedStimulus(t)
	want := []struct {
		user, id string
		stimulus map[string]float64
	}{
		{users.a, "romanian_deadlift", seeded["romanian_deadlift"]},
		{users.a, "deadlift", seeded["deadlift"]},
		{users.b, "romanian_deadlift", seeded["romanian_deadlift"]},
		{users.c, "romanian_deadlift", parseStimulus(t, edited)},
		{users.a, "good_morning", parseStimulus(t, oldRDL)},
	}
	for _, w := range want {
		var raw string
		if err := pool.QueryRow(ctx,
			"SELECT stimulus::text FROM user_exercises WHERE user_id = $1 AND id = $2",
			w.user, w.id).Scan(&raw); err != nil {
			t.Fatalf("%s を読めない: %v", w.id, err)
		}
		if got := parseStimulus(t, raw); !maps.Equal(got, w.stimulus) {
			t.Errorf("%s（%s）の効き方が %v。%v のはず", w.id, w.user[:1], got, w.stimulus)
		}
	}
}

// seedStimulus はシードの種目ごとの効き方を、DB に入る形（区分名 → 寄与）で返す。
func seedStimulus(t *testing.T) map[string]map[string]float64 {
	t.Helper()
	all, err := seed.Exercises()
	if err != nil {
		t.Fatalf("シードが不正: %v", err)
	}
	out := make(map[string]map[string]float64, len(all))
	for _, e := range all {
		m := map[string]float64{}
		for _, r := range e.Stimulus().Regions() {
			c, _ := e.Stimulus().Contribution(r)
			m[string(r)] = c.Float()
		}
		out[string(e.ID())] = m
	}
	return out
}

func parseStimulus(t *testing.T, raw string) map[string]float64 {
	t.Helper()
	var m map[string]float64
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		t.Fatalf("効き方を読めない: %v", err)
	}
	return m
}
