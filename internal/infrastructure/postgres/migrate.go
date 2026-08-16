package postgres

import (
	"context"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"fmt"
	"io/fs"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed migrations/*.sql
var migrationFS embed.FS

// migrationLockKey はマイグレーション用のアドバイザリロックの鍵。
//
// 複数インスタンスが同時に起動しても二重に適用しない。値は任意だが、
// 他の用途のロックと衝突しないよう固定の定数にする。
const migrationLockKey int64 = 8_100_251_121

// Migrate は未適用のマイグレーションを順に流す。
//
// 外部のマイグレーションツールを使わないのは、必要なのが「連番の SQL を
// トランザクションで順に流し、適用済みを記録する」だけだから。それに
// CLI とドライバ登録を持ち込む依存を足す理由がない。
//
// 1ファイルを1トランザクションで流す。途中で失敗したら、そのファイルの
// 変更は丸ごと巻き戻る。半分だけ当たったスキーマは、次の起動で
// 「適用済みでないのに既にある」という直しようのない状態になる。
func Migrate(ctx context.Context, pool *pgxpool.Pool) error {
	conn, err := pool.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("接続を取得できない: %w", err)
	}
	defer conn.Release()

	// ロックは接続に紐づくので、以降は同じ接続で行う。
	if _, err := conn.Exec(ctx, "SELECT pg_advisory_lock($1)", migrationLockKey); err != nil {
		return fmt.Errorf("マイグレーションのロックを取得できない: %w", err)
	}
	defer func() {
		// ロックの解放に失敗しても、接続を返せばセッション終了で外れる。
		_, _ = conn.Exec(context.WithoutCancel(ctx),
			"SELECT pg_advisory_unlock($1)", migrationLockKey)
	}()

	if _, err := conn.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version    text PRIMARY KEY,
			applied_at timestamptz NOT NULL DEFAULT now(),
			checksum   text
		)`); err != nil {
		return fmt.Errorf("適用履歴のテーブルを作れない: %w", err)
	}
	// 適用履歴の形自体はマイグレーションの管理対象にしない。
	// SQL ファイル側で足すと、その SQL を流す前に読み出しが走って失敗する。
	if _, err := conn.Exec(ctx,
		"ALTER TABLE schema_migrations ADD COLUMN IF NOT EXISTS checksum text"); err != nil {
		return fmt.Errorf("適用履歴の形を整えられない: %w", err)
	}

	applied, err := appliedVersions(ctx, conn)
	if err != nil {
		return err
	}

	files, err := migrationFiles()
	if err != nil {
		return err
	}
	for _, f := range files {
		stored, ok := applied[f.version]
		if ok {
			// 適用済みのファイルが書き換えられていないか確かめる。
			// バージョンだけを見ていると、内容を変えても永久に
			// スキップされ、開発と本番がスキーマ違いのまま同じ
			// 「適用済み」を名乗る。
			//
			// 記録が古くてチェックサムが空の場合は、後から埋める。
			switch {
			case stored == "":
				if err := recordChecksum(ctx, conn, f); err != nil {
					return err
				}
			case stored != f.checksum:
				return fmt.Errorf(
					"%s は適用済みだが内容が変わっている（記録 %s / 現在 %s）。"+
						"適用済みのマイグレーションは書き換えず、新しいファイルを足すこと",
					f.version, stored[:8], f.checksum[:8])
			}
			continue
		}
		if err := applyOne(ctx, conn, f); err != nil {
			return err
		}
	}
	return nil
}

func recordChecksum(ctx context.Context, conn interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
}, f migrationFile) error {
	if _, err := conn.Exec(ctx,
		"UPDATE schema_migrations SET checksum = $1 WHERE version = $2",
		f.checksum, f.version); err != nil {
		return fmt.Errorf("%s: チェックサムを記録できない: %w", f.version, err)
	}
	return nil
}

type migrationFile struct {
	version  string
	sql      string
	checksum string
}

// migrationFiles は埋め込んだ SQL をファイル名の昇順で返す。
//
// 順序はスキーマの正しさに直結する。map の反復順に依存すると、
// 依存関係のあるマイグレーションが逆順に流れる。
func migrationFiles() ([]migrationFile, error) {
	entries, err := fs.ReadDir(migrationFS, "migrations")
	if err != nil {
		return nil, fmt.Errorf("マイグレーションを読めない: %w", err)
	}

	out := make([]migrationFile, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".sql") {
			continue
		}
		body, err := migrationFS.ReadFile("migrations/" + e.Name())
		if err != nil {
			return nil, fmt.Errorf("%s を読めない: %w", e.Name(), err)
		}
		sum := sha256.Sum256(body)
		out = append(out, migrationFile{
			version:  strings.TrimSuffix(e.Name(), ".sql"),
			sql:      string(body),
			checksum: hex.EncodeToString(sum[:]),
		})
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("マイグレーションが1つも埋め込まれていない")
	}
	sort.Slice(out, func(i, j int) bool { return out[i].version < out[j].version })
	return out, nil
}

// appliedVersions は適用済みのバージョンとチェックサムを返す。
func appliedVersions(ctx context.Context, conn interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
}) (map[string]string, error) {
	rows, err := conn.Query(ctx,
		"SELECT version, coalesce(checksum, '') FROM schema_migrations")
	if err != nil {
		return nil, fmt.Errorf("適用履歴を読めない: %w", err)
	}
	defer rows.Close()

	out := map[string]string{}
	for rows.Next() {
		var v, sum string
		if err := rows.Scan(&v, &sum); err != nil {
			return nil, fmt.Errorf("適用履歴を読めない: %w", err)
		}
		out[v] = sum
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("適用履歴を読めない: %w", err)
	}
	return out, nil
}

func applyOne(ctx context.Context, conn interface {
	Begin(context.Context) (pgx.Tx, error)
}, f migrationFile) error {
	tx, err := conn.Begin(ctx)
	if err != nil {
		return fmt.Errorf("%s: トランザクションを開始できない: %w", f.version, err)
	}
	defer func() { _ = tx.Rollback(context.WithoutCancel(ctx)) }()

	if _, err := tx.Exec(ctx, f.sql); err != nil {
		return fmt.Errorf("%s: 適用に失敗: %w", f.version, err)
	}
	if _, err := tx.Exec(ctx,
		"INSERT INTO schema_migrations (version, checksum) VALUES ($1, $2)",
		f.version, f.checksum); err != nil {
		return fmt.Errorf("%s: 適用履歴を記録できない: %w", f.version, err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("%s: コミットに失敗: %w", f.version, err)
	}
	return nil
}

// MigrationVersions は埋め込まれたマイグレーションのバージョン一覧。
// テストが件数を直書きせずに済むようにする。
func MigrationVersions() ([]string, error) {
	files, err := migrationFiles()
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(files))
	for _, f := range files {
		out = append(out, f.version)
	}
	return out, nil
}
