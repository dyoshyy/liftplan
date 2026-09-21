package postgres_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/dyoshyy/liftplan/internal/domain/training"
	"github.com/dyoshyy/liftplan/internal/domain/training/condition"
	"github.com/dyoshyy/liftplan/internal/domain/training/setlog"
	"github.com/dyoshyy/liftplan/internal/infrastructure/postgres"
)

// 書き込みの途中で接続が切れたら「保存先に到達できない」として返ること。
//
// Begin や先行する Get で落ちる障害は元から 503 になっていた。ここが守るのは
// その先で、書き込みの文を撃ったあとに backend が落とされた場合。
// ErrRepositoryUnavailable を伴わないと 500 になり、
// 「500 を返すとクライアントは諦める」（maxSaveAttempts のコメント）と食い違う。
//
// 切断の作り方：別のトランザクションで対象の行を締めて Save をロック待ちに
// してから、待っている backend を pg_terminate_backend で落とす（57P01）。
func TestSave_ReportsUnavailableWhenDisconnectedMidWrite(t *testing.T) {
	cases := []struct {
		name string
		// table は pg_stat_activity から待っている文を見つけるための目印。
		table string
		// what は出口で付く文言。二重に包むと2回現れる。
		what string
		// hold は Save を待たせる行ロックを tx の中で取る。
		hold func(t *testing.T, pool *pgxpool.Pool, tx pgx.Tx)
		save func(pool *pgxpool.Pool) error
	}{
		{
			// ON CONFLICT DO NOTHING は確定済みの行なら待たずに読み飛ばす。
			// 待たせるには、同じ主キーの INSERT を未確定のまま置く。
			name: "実績", table: "set_logs", what: "実績を保存できない",
			hold: func(t *testing.T, _ *pgxpool.Pool, tx pgx.Tx) {
				if _, err := tx.Exec(context.Background(), `
					INSERT INTO set_logs (user_id, id, performed_on, exercise_id, weight_kg, reps, rir)
					VALUES ($1, 'blocked', '2026-08-17', 'bench', 100, 9, 2)`,
					userA(t).String()); err != nil {
					t.Fatalf("行を締められない: %v", err)
				}
			},
			save: func(pool *pgxpool.Pool) error {
				return postgres.NewSetLogRepository(pool).Save(context.Background(),
					userA(t), []*setlog.SetLog{mkSetLog(t, "blocked", 100)})
			},
		},
		{
			name: "コンディション", table: "daily_conditions", what: "コンディションを保存できない",
			hold: func(t *testing.T, pool *pgxpool.Pool, tx pgx.Tx) {
				if err := postgres.NewConditionRepository(pool).Save(context.Background(),
					userA(t), []condition.DailyCondition{
						condition.NewDailyCondition(day).WithBodyWeight(70),
					}); err != nil {
					t.Fatalf("下準備の保存に失敗: %v", err)
				}
				if _, err := tx.Exec(context.Background(),
					"SELECT 1 FROM daily_conditions WHERE user_id = $1 FOR UPDATE",
					userA(t).String()); err != nil {
					t.Fatalf("行を締められない: %v", err)
				}
			},
			save: func(pool *pgxpool.Pool) error {
				return postgres.NewConditionRepository(pool).Save(context.Background(),
					userA(t), []condition.DailyCondition{
						condition.NewDailyCondition(day).WithBodyWeight(71),
					})
			},
		},
		{
			name: "プログラム", table: "program", what: "プログラムを保存できない",
			hold: func(t *testing.T, pool *pgxpool.Pool, tx pgx.Tx) {
				if err := postgres.NewProgramRepository(pool).Save(context.Background(),
					userA(t), samplePrograms(t)); err != nil {
					t.Fatalf("下準備の保存に失敗: %v", err)
				}
				if _, err := tx.Exec(context.Background(),
					"SELECT 1 FROM program WHERE user_id = $1 FOR UPDATE",
					userA(t).String()); err != nil {
					t.Fatalf("行を締められない: %v", err)
				}
			},
			save: func(pool *pgxpool.Pool) error {
				return postgres.NewProgramRepository(pool).Save(context.Background(),
					userA(t), samplePrograms(t))
			},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			pool := migratedDB(t)
			ctx := context.Background()

			tx, err := pool.Begin(ctx)
			if err != nil {
				t.Fatalf("トランザクションを開始できない: %v", err)
			}
			// スキーマの掃除（t.Cleanup）より先に放す。締めたままだと
			// DROP SCHEMA が待ち続ける。
			defer func() { _ = tx.Rollback(ctx) }()
			c.hold(t, pool, tx)

			done := make(chan error, 1)
			go func() { done <- c.save(pool) }()

			terminateBlockedInsert(t, pool, c.table)

			var saveErr error
			select {
			case saveErr = <-done:
			case <-time.After(10 * time.Second):
				t.Fatal("Save が戻らない")
			}

			if saveErr == nil {
				t.Fatal("接続を切ったのに保存が成功している")
			}
			if !errors.Is(saveErr, training.ErrRepositoryUnavailable) {
				t.Errorf("ErrRepositoryUnavailable を伴っていない: %v", saveErr)
			}
			if n := strings.Count(saveErr.Error(), c.what); n != 1 {
				t.Errorf("文言 %q が %d 回現れる（1回のはず）: %v", c.what, n, saveErr)
			}
		})
	}
}

// 接続できないときも、到達できない旨が1度だけ付くこと。
//
// Begin の失敗は元から包まれていた。出口で包む形にしたので、中でも包むと
// ErrRepositoryUnavailable の文言が二重になる。ProgramRepository.Save は
// Begin を持たず、元は接続拒否でも 500 だった。
//
// DB が要らない（誰も聞いていないポートへ繋ぐ）ので、TEST_DATABASE_URL が
// 無くても走る。
func TestSave_ReportsUnavailableOnceWhenUnreachable(t *testing.T) {
	// postgres.Open は疎通を確かめて失敗するので、プールを直接作る。
	pool, err := pgxpool.New(context.Background(),
		"postgres://postgres@127.0.0.1:1/postgres?connect_timeout=2")
	if err != nil {
		t.Fatalf("プールを作れない: %v", err)
	}
	t.Cleanup(pool.Close)
	ctx := context.Background()

	cases := []struct {
		name string
		save func() error
	}{
		{name: "実績", save: func() error {
			return postgres.NewSetLogRepository(pool).Save(ctx,
				userA(t), []*setlog.SetLog{mkSetLog(t, "x", 100)})
		}},
		{name: "コンディション", save: func() error {
			return postgres.NewConditionRepository(pool).Save(ctx,
				userA(t), []condition.DailyCondition{
					condition.NewDailyCondition(day).WithBodyWeight(70),
				})
		}},
		{name: "プログラム", save: func() error {
			return postgres.NewProgramRepository(pool).Save(ctx, userA(t), samplePrograms(t))
		}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := c.save()

			if !errors.Is(err, training.ErrRepositoryUnavailable) {
				t.Fatalf("ErrRepositoryUnavailable を伴っていない: %v", err)
			}
			sentinel := training.ErrRepositoryUnavailable.Error()
			if n := strings.Count(err.Error(), sentinel); n != 1 {
				t.Errorf("%q が %d 回現れる（1回のはず）: %v", sentinel, n, err)
			}
		})
	}
}

// terminateBlockedInsert は table への INSERT でロック待ちになっている
// backend を見つけて落とす。
func terminateBlockedInsert(t *testing.T, pool *pgxpool.Pool, table string) {
	t.Helper()
	ctx := context.Background()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		var killed int
		if err := pool.QueryRow(ctx, `
			SELECT count(*) FROM (
				SELECT pg_terminate_backend(pid)
				FROM pg_stat_activity
				WHERE pid <> pg_backend_pid()
				  AND wait_event_type = 'Lock'
				  AND query LIKE '%INSERT INTO ' || $1 || ' %'
			) AS t`, table).Scan(&killed); err != nil {
			t.Fatalf("待っている backend を探せない: %v", err)
		}
		if killed > 0 {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("%s への INSERT がロック待ちにならなかった", table)
}
