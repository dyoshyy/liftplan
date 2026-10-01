package postgres_test

import (
	"context"
	"os"
	"testing"

	"github.com/dyoshyy/liftplan/internal/infrastructure/postgres"
)

// 消した種目は、非表示の種目として戻る。
//
// 0015 を流す前の状態を作るため、全部のマイグレーションを当てたあとで行を
// 入れ、0015 の SQL をもう一度流す（UPDATE だけなので何度流しても同じ）。
func TestMigration0015_RestoresDeletedExercises(t *testing.T) {
	pool := newTestDB(t)
	ctx := context.Background()
	if err := postgres.Migrate(ctx, pool); err != nil {
		t.Fatalf("マイグレーションに失敗: %v", err)
	}
	sql, err := os.ReadFile("migrations/0015_restore_deleted_exercises.sql")
	if err != nil {
		t.Fatalf("0015 を読めない: %v", err)
	}

	const user = "11111111-1111-1111-1111-111111111111"
	rows := []struct {
		id, name string
		deleted  string // 空なら消していない
	}{
		// 同名の消していない行が無い → 戻る。
		{"gone", "消した種目", "2026-09-01"},
		// 同名の消していない行がある → 戻さない（名前の一意索引に当たる）。
		{"dup-old", "同名", "2026-09-01"},
		{"dup-alive", "同名", ""},
		// 消した同名が2行 → 新しいほうだけ戻る。
		{"twice-old", "二重", "2026-08-01"},
		{"twice-new", "二重", "2026-09-01"},
		// 消していない行はそのまま。
		{"alive", "生きている", ""},
	}
	for _, r := range rows {
		deleted := any(nil)
		if r.deleted != "" {
			deleted = r.deleted
		}
		if _, err := pool.Exec(ctx, `
			INSERT INTO user_exercises (user_id, id, name, stimulus, increment_kg, deleted_at)
			VALUES ($1, $2, $3, '{"LAT": 1.0}', 2.5, $4::timestamptz)`,
			user, r.id, r.name, deleted); err != nil {
			t.Fatalf("%s を入れられない: %v", r.id, err)
		}
	}

	if _, err := pool.Exec(ctx, string(sql)); err != nil {
		t.Fatalf("0015 が失敗: %v", err)
	}

	want := map[string]bool{ // true = 消した印が残る
		"gone": false, "dup-old": true, "dup-alive": false,
		"twice-old": true, "twice-new": false, "alive": false,
	}
	for id, stillDeleted := range want {
		var deleted bool
		if err := pool.QueryRow(ctx,
			"SELECT deleted_at IS NOT NULL FROM user_exercises WHERE user_id = $1 AND id = $2",
			user, id).Scan(&deleted); err != nil {
			t.Fatalf("%s を読めない: %v", id, err)
		}
		if deleted != stillDeleted {
			t.Errorf("%s: 消した印が %v。%v のはず", id, deleted, stillDeleted)
		}
	}
}
