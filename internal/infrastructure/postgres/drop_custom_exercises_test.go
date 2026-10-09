package postgres_test

import (
	"context"
	"os"
	"testing"

	"github.com/dyoshyy/liftplan/internal/infrastructure/postgres"
)

// 旧版の種目の表（0013 の custom_exercises）を消す（0019）。取り込み漏れの行が
// 1件でもあれば、消さずに止まる。
//
// 旧版で足した種目は、初回の読み出しで user_exercises に取り込んでいた
// （以前の TestExerciseRepository_Postgres_ImportsLegacyCustomExercises が見ていた）。
// 本番では全員が取り込み済みになったので、取り込みの SQL ごと表を消す。表を消すと
// 元に戻せないので、取り込み漏れの行があれば消さずに失敗させる。サーバーは起動時に
// マイグレーションを流すので、失敗すれば Deploy のヘルスチェックで止まる。
//
// 0019 を流す前の状態を作るため、全部のマイグレーションを当てたあとで 0013 の
// 表を作り直し、0019 の SQL をもう一度流す。
func TestMigration0019_DropsLegacyTableOnlyWhenEveryRowIsImported(t *testing.T) {
	pool := newTestDB(t)
	ctx := context.Background()
	if err := postgres.Migrate(ctx, pool); err != nil {
		t.Fatalf("マイグレーションに失敗: %v", err)
	}
	create, err := os.ReadFile("migrations/0013_custom_exercises.sql")
	if err != nil {
		t.Fatalf("0013 を読めない: %v", err)
	}
	drop, err := os.ReadFile("migrations/0019_drop_custom_exercises.sql")
	if err != nil {
		t.Fatalf("0019 を読めない: %v", err)
	}
	if _, err := pool.Exec(ctx, string(create)); err != nil {
		t.Fatalf("旧版の表を作り直せない: %v", err)
	}
	exists := func() bool {
		var ok bool
		if err := pool.QueryRow(ctx, "SELECT to_regclass('custom_exercises') IS NOT NULL").Scan(&ok); err != nil {
			t.Fatalf("custom_exercises の存在を確認できない: %v", err)
		}
		return ok
	}

	const user = "11111111-1111-1111-1111-111111111111"
	if _, err := pool.Exec(ctx, `
		INSERT INTO custom_exercises (user_id, id, name, primary_regions, secondary_regions, increment_kg)
		VALUES ($1, 'u-legacy', '旧版のマシン', '["LAT"]', '[]', 2.5)`, user); err != nil {
		t.Fatalf("旧版の行を入れられない: %v", err)
	}

	// 取り込み漏れがある → 失敗して、表は残る
	if _, err := pool.Exec(ctx, string(drop)); err == nil {
		t.Fatal("取り込み漏れの行があるのに 0019 が通った")
	}
	if !exists() {
		t.Fatal("取り込み漏れの行があるのに表が消えた")
	}

	// 取り込み済み → 表が消える
	if _, err := pool.Exec(ctx, `
		INSERT INTO user_exercises (user_id, id, name, stimulus, increment_kg)
		VALUES ($1, 'u-legacy', '旧版のマシン', '{"LAT": 1.0}'::jsonb, 2.5)`, user); err != nil {
		t.Fatalf("取り込んだ行を入れられない: %v", err)
	}
	if _, err := pool.Exec(ctx, string(drop)); err != nil {
		t.Fatalf("取り込み済みなのに 0019 が失敗: %v", err)
	}
	if exists() {
		t.Error("取り込み済みなのに表が残っている")
	}
}
