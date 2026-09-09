package postgres_test

import (
	"context"
	"strings"
	"sync"
	"testing"

	"github.com/dyoshyy/liftplan/internal/infrastructure/postgres"
)

func TestMigrate_CreatesEverySchemaObject(t *testing.T) {
	pool := newTestDB(t)
	ctx := context.Background()

	if err := postgres.Migrate(ctx, pool); err != nil {
		t.Fatalf("マイグレーションに失敗: %v", err)
	}

	for _, table := range []string{"set_logs", "daily_conditions", "program", "schema_migrations"} {
		var exists bool
		if err := pool.QueryRow(ctx,
			"SELECT to_regclass($1) IS NOT NULL", table).Scan(&exists); err != nil {
			t.Fatalf("%s の存在を確認できない: %v", table, err)
		}
		if !exists {
			t.Errorf("%s が作られていない", table)
		}
	}

	// 0002 で落とした索引が残っていないこと。
	//
	// set_logs を読む SQL は FindAll の1本だけで WHERE 句が無く、
	// 日付や種目での絞り込みは Go 側で行う。走査されない索引は
	// INSERT ごとの更新コストを払うだけになる。
	for _, index := range []string{"set_logs_performed_on_idx", "set_logs_exercise_idx"} {
		var exists bool
		if err := pool.QueryRow(ctx,
			"SELECT to_regclass($1) IS NOT NULL", index).Scan(&exists); err != nil {
			t.Fatalf("%s の存在を確認できない: %v", index, err)
		}
		if exists {
			t.Errorf("使われない索引 %s が残っている", index)
		}
	}
}

// 二度流しても壊れないこと。起動のたびに呼ぶので冪等でなければならない。
func TestMigrate_IsIdempotent(t *testing.T) {
	pool := newTestDB(t)
	ctx := context.Background()

	for i := range 3 {
		if err := postgres.Migrate(ctx, pool); err != nil {
			t.Fatalf("%d回目のマイグレーションに失敗: %v", i+1, err)
		}
	}

	var count int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM schema_migrations").Scan(&count); err != nil {
		t.Fatalf("適用履歴を読めない: %v", err)
	}
	if count != len(migrationCount(t)) {
		t.Errorf("適用履歴の件数が誤り: %d（期待 %d）", count, len(migrationCount(t)))
	}
}

// 同時に起動しても二重に適用しないこと。
func TestMigrate_IsSafeForConcurrentStartup(t *testing.T) {
	pool := migratedDB(t)
	ctx := context.Background()

	// 一度掃除してから、同時に流す。
	if _, err := pool.Exec(ctx, `
		DROP TABLE IF EXISTS set_logs, daily_conditions, program, schema_migrations CASCADE`); err != nil {
		t.Fatalf("掃除できない: %v", err)
	}

	var wg sync.WaitGroup
	errs := make([]error, 8)
	for i := range errs {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			errs[i] = postgres.Migrate(ctx, pool)
		}(i)
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Errorf("%d番目の起動が失敗: %v", i, err)
		}
	}

	var count int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM schema_migrations").Scan(&count); err != nil {
		t.Fatalf("適用履歴を読めない: %v", err)
	}
	if count != len(migrationCount(t)) {
		t.Errorf("二重に適用された: %d件（期待 %d）", count, len(migrationCount(t)))
	}
}

// 適用に失敗したら、そのファイルの変更が丸ごと巻き戻ること。
// 半分だけ当たったスキーマは、次の起動で「適用済みでないのに既にある」
// という直しようのない状態になる。
func TestMigrate_RollsBackOnFailure(t *testing.T) {
	pool := newTestDB(t)
	ctx := context.Background()

	// set_logs だけを先に作っておくと、0001 の途中で衝突する。
	if _, err := pool.Exec(ctx, "CREATE TABLE set_logs (id text)"); err != nil {
		t.Fatalf("準備に失敗: %v", err)
	}

	if err := postgres.Migrate(ctx, pool); err == nil {
		t.Fatal("衝突しているのに成功した")
	}

	// 巻き戻っていれば、後続のテーブルは作られていない。
	for _, table := range []string{"daily_conditions", "program"} {
		var exists bool
		if err := pool.QueryRow(ctx,
			"SELECT to_regclass($1) IS NOT NULL", table).Scan(&exists); err != nil {
			t.Fatalf("%s の存在を確認できない: %v", table, err)
		}
		if exists {
			t.Errorf("失敗したのに %s が残っている", table)
		}
	}

	// 適用済みとして記録されていないこと。記録されると、
	// 次の起動で「流したことになっているのに実体が無い」状態になる。
	var count int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM schema_migrations").Scan(&count); err != nil {
		t.Fatalf("適用履歴を読めない: %v", err)
	}
	if count != 0 {
		t.Errorf("失敗したのに適用済みとして記録された: %d件", count)
	}
}

func TestOpen_RejectsUnreachableDatabase(t *testing.T) {
	_, err := postgres.Open(context.Background(),
		"postgres://nobody:nobody@127.0.0.1:1/nothing")
	if err == nil {
		t.Error("到達できない接続先で成功した")
	}
}

func TestOpen_RejectsMalformedURL(t *testing.T) {
	if _, err := postgres.Open(context.Background(), "これは接続文字列ではない"); err == nil {
		t.Error("不正な接続文字列で成功した")
	}
}

// migrationCount は埋め込まれたマイグレーションの一覧。
// 件数を直書きすると、ファイルを足すたびにテストが落ちる。
func migrationCount(t *testing.T) []string {
	t.Helper()
	names, err := postgres.MigrationVersions()
	if err != nil {
		t.Fatalf("マイグレーションを列挙できない: %v", err)
	}
	return names
}

// 適用済みのマイグレーションを書き換えたら気づくこと。
//
// バージョンだけを見ていると、内容を変えても永久にスキップされ、
// 開発と本番がスキーマ違いのまま同じ「適用済み」を名乗る。
func TestMigrate_DetectsRewrittenMigration(t *testing.T) {
	pool := migratedDB(t)
	ctx := context.Background()

	if _, err := pool.Exec(ctx,
		"UPDATE schema_migrations SET checksum = 'ちがう内容のチェックサム'"); err != nil {
		t.Fatalf("準備に失敗: %v", err)
	}

	err := postgres.Migrate(ctx, pool)
	if err == nil {
		t.Fatal("書き換えを検出できていない")
	}
	if !strings.Contains(err.Error(), "内容が変わっている") {
		t.Errorf("原因が読めないエラー: %v", err)
	}
}

// チェックサムの記録が無い古い DB でも動くこと。
func TestMigrate_BackfillsMissingChecksum(t *testing.T) {
	pool := migratedDB(t)
	ctx := context.Background()

	if _, err := pool.Exec(ctx, "UPDATE schema_migrations SET checksum = NULL"); err != nil {
		t.Fatalf("準備に失敗: %v", err)
	}
	if err := postgres.Migrate(ctx, pool); err != nil {
		t.Fatalf("チェックサムが無い DB で失敗: %v", err)
	}

	var missing int
	if err := pool.QueryRow(ctx,
		"SELECT count(*) FROM schema_migrations WHERE checksum IS NULL").Scan(&missing); err != nil {
		t.Fatalf("確認できない: %v", err)
	}
	if missing != 0 {
		t.Errorf("チェックサムが埋められていない: %d件", missing)
	}
}
