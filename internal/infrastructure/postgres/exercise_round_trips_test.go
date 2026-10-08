package postgres_test

import (
	"context"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/dyoshyy/liftplan/internal/domain/training/seed"
	"github.com/dyoshyy/liftplan/internal/infrastructure/postgres"
)

// statementCounter は DB に投げた文を控える。BEGIN・ROLLBACK も1文として数える。
type statementCounter struct {
	mu   sync.Mutex
	sqls []string
}

func (c *statementCounter) TraceQueryStart(ctx context.Context, _ *pgx.Conn, d pgx.TraceQueryStartData) context.Context {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.sqls = append(c.sqls, strings.Join(strings.Fields(d.SQL), " "))
	return ctx
}

func (c *statementCounter) TraceQueryEnd(context.Context, *pgx.Conn, pgx.TraceQueryEndData) {}

func (c *statementCounter) reset() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.sqls = nil
}

func (c *statementCounter) taken() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]string(nil), c.sqls...)
}

// tracedDB は migratedDB と同じスキーマを、文を数える接続で開き直す。
func tracedDB(t *testing.T) (*pgxpool.Pool, *statementCounter) {
	t.Helper()
	migratedDB(t)
	cfg, err := pgxpool.ParseConfig(withSearchPath(os.Getenv(dbEnv), "test_"+sanitize(t.Name())))
	if err != nil {
		t.Fatal(err)
	}
	counter := &statementCounter{}
	cfg.ConnConfig.Tracer = counter
	pool, err := pgxpool.NewWithConfig(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	return pool, counter
}

// シードが入っている利用者の読み出し・保存は、シードの判定にトランザクションを
// 開かない（#227）。
//
// 計画の導出は毎回種目一覧を読むので、判定に BEGIN → count → ROLLBACK の3往復を
// 足すと、1回の読み出しの往復が倍になる。トランザクションが要るのはシードを
// 入れるときだけで、入っているかの判定は1文で足りる。
func TestExerciseRepository_Postgres_SeededUserSkipsTheSeedTransaction(t *testing.T) {
	ctx := context.Background()
	seedAll, err := seed.Exercises()
	if err != nil {
		t.Fatalf("シードが不正: %v", err)
	}
	pool, counter := tracedDB(t)
	repo := postgres.NewExerciseRepository(pool, seedAll)
	a := newUser(t)

	if _, err := repo.FindAll(ctx, a); err != nil {
		t.Fatalf("初回の取得に失敗: %v", err)
	}

	counter.reset()
	if _, err := repo.FindAll(ctx, a); err != nil {
		t.Fatalf("2回目の取得に失敗: %v", err)
	}
	// 入っているかの確認1文と、一覧の読み出し1文
	if got := counter.taken(); len(got) != 2 {
		t.Errorf("読み出しで %d 文を投げた。2文のはず:\n%s", len(got), strings.Join(got, "\n"))
	}

	counter.reset()
	if err := repo.Save(ctx, a, seedAll[0]); err != nil {
		t.Fatalf("保存に失敗: %v", err)
	}
	// 入っているかの確認1文と、upsert 1文
	if got := counter.taken(); len(got) != 2 {
		t.Errorf("保存で %d 文を投げた。2文のはず:\n%s", len(got), strings.Join(got, "\n"))
	}
}
