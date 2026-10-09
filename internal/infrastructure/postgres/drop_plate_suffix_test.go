package postgres_test

import (
	"context"
	"os"
	"testing"

	"github.com/dyoshyy/liftplan/internal/infrastructure/postgres"
)

// Hammer Strength のプリセットの名前から「（プレート）」を外す（0018）。
//
// 種目は利用者ごとにコピーされている（0014）ので、シードを直すだけでは既存の
// 利用者に届かない。0017 と同じく、全部のマイグレーションを当てたあとで行を
// 入れ、0018 の SQL をもう一度流す（条件付きの UPDATE だけなので何度流しても同じ）。
func TestMigration0018_DropsPlateSuffix(t *testing.T) {
	pool := newTestDB(t)
	ctx := context.Background()
	if err := postgres.Migrate(ctx, pool); err != nil {
		t.Fatalf("マイグレーションに失敗: %v", err)
	}
	sql, err := os.ReadFile("migrations/0018_drop_plate_suffix.sql")
	if err != nil {
		t.Fatalf("0018 を読めない: %v", err)
	}

	const user = "11111111-1111-1111-1111-111111111111"
	rows := []struct {
		id, name string
		deleted  bool
	}{
		// シードのままの名前 → 外れる
		{"hs_pl_lateral_raise", "HS ラテラルレイズ（プレート）", false},
		// 消した行も外す。戻したときに古い名前が蘇らないように
		{"hs_pl_hack_squat", "HS ハックスクワット（プレート）", true},
		// 本人が付け直した名前 → 触らない
		{"hs_pl_iso_row", "ローマシン", false},
		// 外した名前が、消していない別の種目と同名になる → 触らない
		// （名前の一意索引に当たって、マイグレーションごと失敗するのを避ける）
		{"hs_pl_glute_drive", "HS グルート・ドライブ（プレート）", false},
		{"u-mine", "HS グルート・ドライブ", false},
		// HS 以外の種目は、名前が「（プレート）」で終わっていても触らない
		{"u-other", "自作レッグプレス（プレート）", false},
	}
	for _, r := range rows {
		deleted := any(nil)
		if r.deleted {
			deleted = "2026-09-01"
		}
		if _, err := pool.Exec(ctx, `
			INSERT INTO user_exercises (user_id, id, name, stimulus, increment_kg, deleted_at)
			VALUES ($1, $2, $3, '{"LAT": 1.0}'::jsonb, 2.5, $4::timestamptz)`,
			user, r.id, r.name, deleted); err != nil {
			t.Fatalf("%s を入れられない: %v", r.id, err)
		}
	}

	if _, err := pool.Exec(ctx, string(sql)); err != nil {
		t.Fatalf("0018 が失敗: %v", err)
	}

	want := map[string]string{
		"hs_pl_lateral_raise": "HS ラテラルレイズ",
		"hs_pl_hack_squat":    "HS ハックスクワット",
		"hs_pl_iso_row":       "ローマシン",
		"hs_pl_glute_drive":   "HS グルート・ドライブ（プレート）",
		"u-mine":              "HS グルート・ドライブ",
		"u-other":             "自作レッグプレス（プレート）",
	}
	for id, w := range want {
		var got string
		if err := pool.QueryRow(ctx,
			"SELECT name FROM user_exercises WHERE user_id = $1 AND id = $2", user, id).Scan(&got); err != nil {
			t.Fatalf("%s を読めない: %v", id, err)
		}
		if got != w {
			t.Errorf("%s の名前が %q。%q のはず", id, got, w)
		}
	}
}
