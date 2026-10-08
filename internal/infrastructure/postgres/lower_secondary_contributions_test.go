package postgres_test

import (
	"context"
	"os"
	"testing"

	"github.com/dyoshyy/liftplan/internal/infrastructure/postgres"
)

// プレスの側部三角筋と引く種目の二頭筋の副次が、シードの旧値のままの行だけ
// 0.3 に下がる。本人が直した行と、対象でない種目は触らない。
//
// 0017 を流す前の状態を作るため、全部のマイグレーションを当てたあとで行を
// 入れ、0017 の SQL をもう一度流す（条件付きの UPDATE だけなので何度流しても同じ）。
func TestMigration0017_LowersSecondaryDeltAndBiceps(t *testing.T) {
	pool := newTestDB(t)
	ctx := context.Background()
	if err := postgres.Migrate(ctx, pool); err != nil {
		t.Fatalf("マイグレーションに失敗: %v", err)
	}
	sql, err := os.ReadFile("migrations/0017_lower_secondary_delt_and_biceps.sql")
	if err != nil {
		t.Fatalf("0017 を読めない: %v", err)
	}

	const user = "11111111-1111-1111-1111-111111111111"
	rows := []struct {
		id, stimulus string
		deleted      bool
	}{
		// シードの旧値のまま → 下がる。
		{"overhead_press", `{"FRONT_DELT": 1.0, "SIDE_DELT": 0.5, "TRICEPS_LATERAL": 0.4}`, false},
		{"pull_up", `{"LAT": 1.0, "BICEPS": 0.5, "FOREARM": 0.3}`, false},
		{"lat_pulldown", `{"LAT": 1.0, "BICEPS": 0.4}`, false},
		// 消した行も下がる。戻したときに旧値が蘇らないように。
		{"hs_pl_iso_row", `{"TRAP_MID": 1.0, "BICEPS": 0.5}`, true},
		// 本人が直した行（旧値と違う）→ 触らない。
		{"db_shoulder_press", `{"FRONT_DELT": 1.0, "SIDE_DELT": 0.7}`, false},
		// 対象でない種目は、同じ値でも触らない。
		{"side_raise", `{"SIDE_DELT": 1.0, "TRAP_UPPER": 0.3}`, false},
		{"barbell_row", `{"TRAP_MID": 1.0, "BICEPS": 0.5}`, false},
	}
	for _, r := range rows {
		deleted := any(nil)
		if r.deleted {
			deleted = "2026-09-01"
		}
		if _, err := pool.Exec(ctx, `
			INSERT INTO user_exercises (user_id, id, name, stimulus, increment_kg, deleted_at)
			VALUES ($1, $2, $2, $3::jsonb, 2.5, $4::timestamptz)`,
			user, r.id, r.stimulus, deleted); err != nil {
			t.Fatalf("%s を入れられない: %v", r.id, err)
		}
	}

	if _, err := pool.Exec(ctx, string(sql)); err != nil {
		t.Fatalf("0017 が失敗: %v", err)
	}

	want := map[string]struct {
		region string
		value  float64
	}{
		"overhead_press":    {"SIDE_DELT", 0.3},
		"pull_up":           {"BICEPS", 0.3},
		"lat_pulldown":      {"BICEPS", 0.3},
		"hs_pl_iso_row":     {"BICEPS", 0.3},
		"db_shoulder_press": {"SIDE_DELT", 0.7},
		"side_raise":        {"SIDE_DELT", 1.0},
		"barbell_row":       {"BICEPS", 0.5},
	}
	for id, w := range want {
		var got float64
		if err := pool.QueryRow(ctx,
			"SELECT (stimulus->>$3)::numeric FROM user_exercises WHERE user_id = $1 AND id = $2",
			user, id, w.region).Scan(&got); err != nil {
			t.Fatalf("%s を読めない: %v", id, err)
		}
		if got != w.value {
			t.Errorf("%s の %s が %v。%v のはず", id, w.region, got, w.value)
		}
	}

	// 主働の寄与は動かない。
	var lat float64
	if err := pool.QueryRow(ctx,
		"SELECT (stimulus->>'LAT')::numeric FROM user_exercises WHERE user_id = $1 AND id = 'pull_up'",
		user).Scan(&lat); err != nil {
		t.Fatalf("pull_up を読めない: %v", err)
	}
	if lat != 1.0 {
		t.Errorf("pull_up の LAT が %v に動いた", lat)
	}
}
