package postgres_test

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/dyoshyy/liftplan-server/internal/infrastructure/postgres"
)

// TEST_DATABASE_URL が無いときの扱い。
//
// 黙ってスキップすると「テストが通った」と「テストを走らせていない」が
// 区別できなくなる。これは D-051 で潰したのと同じ形の失敗なので、
// スキップの理由を必ず出す。CI では設定を必須にする。
const dbEnv = "TEST_DATABASE_URL"

var (
	basePoolOnce sync.Once
	basePool     *pgxpool.Pool
	basePoolErr  error
)

// newTestDB はテストごとに独立したスキーマを持つ接続プールを返す。
//
// データベースごと分けないのは、作成に時間がかかるため。スキーマを
// 分ければテーブル名は衝突せず、search_path で切り替えられる。
func newTestDB(t *testing.T) *pgxpool.Pool {
	t.Helper()

	url := os.Getenv(dbEnv)
	if url == "" {
		t.Skipf("%s が未設定のため実行しない。"+
			"`docker run -d -e POSTGRES_PASSWORD=x -p 5432:5432 postgres:17-alpine` などで用意し、"+
			"%s=postgres://postgres:x@127.0.0.1:5432/postgres を設定すること", dbEnv, dbEnv)
	}

	// pooler のエンドポイントは search_path を startup で受け付けない
	// （"unsupported startup parameter in options: search_path"）。
	// スキーマで分離できないので、黙って共有スキーマに落ちるのではなく止める。
	// 共有されたまま走ると、あるテストが壊した schema_migrations を
	// 他のテストが踏み、原因の分からない失敗が並ぶ。
	if strings.Contains(url, "-pooler.") {
		t.Fatalf("%s が pooler を指している。テストはスキーマで分離するので"+
			"プーラー無しのエンドポイントを使うこと（ホスト名から -pooler を外す）", dbEnv)
	}

	ctx := context.Background()
	basePoolOnce.Do(func() { basePool, basePoolErr = postgres.Open(ctx, url) })
	if basePoolErr != nil {
		t.Fatalf("接続できない: %v", basePoolErr)
	}

	schema := "test_" + sanitize(t.Name())
	if _, err := basePool.Exec(ctx,
		fmt.Sprintf("DROP SCHEMA IF EXISTS %s CASCADE", schema)); err != nil {
		t.Fatalf("スキーマを掃除できない: %v", err)
	}
	if _, err := basePool.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatalf("スキーマを作れない: %v", err)
	}
	t.Cleanup(func() {
		_, _ = basePool.Exec(context.Background(),
			fmt.Sprintf("DROP SCHEMA IF EXISTS %s CASCADE", schema))
	})

	pool, err := postgres.Open(ctx, withSearchPath(url, schema))
	if err != nil {
		t.Fatalf("スキーマ付きで接続できない: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

// migratedDB はマイグレーション済みのテスト用データベースを返す。
func migratedDB(t *testing.T) *pgxpool.Pool {
	t.Helper()
	pool := newTestDB(t)
	if err := postgres.Migrate(context.Background(), pool); err != nil {
		t.Fatalf("マイグレーションに失敗: %v", err)
	}
	return pool
}

// withSearchPath は接続文字列に search_path を足す。
//
// `?search_path=` ではなく `options=-c search_path=` を使う。前者は libpq の
// パラメータではなく（psql は "invalid URI query parameter" で拒否する）、
// pgx が独自に起動パラメータとして送っているだけ。ローカルの Postgres には
// 素通しで届くが、Neon のプロキシは通さない。
//
// その結果どうなるかというと、**全てのテストが同じスキーマを共有する**。
// スキーマを分けたつもりで分かれておらず、あるテストが壊した
// schema_migrations を他のテストが踏む。ローカルでは緑のままなので、
// 実際の Neon に流すまで気づけなかった。
func withSearchPath(rawURL, schema string) string {
	sep := "?"
	if strings.Contains(rawURL, "?") {
		sep = "&"
	}
	return rawURL + sep + "options=" + url.QueryEscape("-c search_path="+schema)
}

// sanitize はテスト名を識別子に使える形にする。
func sanitize(name string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(name) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	return b.String()
}
