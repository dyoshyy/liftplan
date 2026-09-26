// Package postgres はリポジトリの Postgres 実装。
//
// pgx を直接使い database/sql を挟まない。差し替え点はリポジトリ
// インターフェースであって SQL ドライバではないので、database/sql の抽象は
// 何も守ってくれない。挟むと prepared statement のキャッシュと型の扱いを
// 捨てることになる。
package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	// connectTimeout は起動時の疎通確認の上限。
	// 無制限にすると、接続先を間違えたときにサーバーが黙って上がらない。
	connectTimeout = 10 * time.Second

	// maxConns は1インスタンスが張る接続の上限。
	//
	// 効いているのは利用者の数ではなく、DB 側の接続数の上限。全体では
	// 「インスタンス数 × maxConns」本になり、Cloud Run の --max-instances=2
	// （docs/deploy.md）と組で 16 本に収めている。片方を動かすなら
	// もう片方も見ること。
	//
	// 増やさないのは、足りないときと余ったときで害が釣り合わないため。
	// 1リクエストは問い合わせを順に投げるので掴むのは同時に1本で、
	// プールが足りなければ空くのを待つだけで済む。DB 側の上限を超えると
	// 接続そのものが拒まれ、利用者の全員が巻き込まれる。
	maxConns = 8

	// maxConnIdleTime は遊休接続を切るまでの時間。
	// サーバーレスの Postgres は接続を長く抱えると課金対象の時間が伸びる。
	maxConnIdleTime = 5 * time.Minute
)

// Open は接続プールを作り、疎通を確認する。
//
// 疎通確認をここでやるのは、接続先の誤りを起動時に落とすため。
// 遅延させると、最初のリクエストが来るまで気づけない。
func Open(ctx context.Context, url string) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		return nil, fmt.Errorf("接続文字列を解釈できない: %w", err)
	}
	cfg.MaxConns = maxConns
	cfg.MaxConnIdleTime = maxConnIdleTime

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("接続プールを作れない: %w", err)
	}

	pingCtx, cancel := context.WithTimeout(ctx, connectTimeout)
	defer cancel()
	if err := pool.Ping(pingCtx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("データベースに疎通できない: %w", err)
	}
	return pool, nil
}
