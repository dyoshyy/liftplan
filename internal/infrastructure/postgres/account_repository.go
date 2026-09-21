package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgerrcode"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/dyoshyy/liftplan/internal/domain/account"
)

// AccountRepository はアカウントの Postgres 実装。
type AccountRepository struct {
	pool *pgxpool.Pool
}

func NewAccountRepository(pool *pgxpool.Pool) *AccountRepository {
	return &AccountRepository{pool: pool}
}

// Find は (provider, subject) のアカウントを返す。
func (r *AccountRepository) Find(
	ctx context.Context, provider account.Provider, subject string,
) (*account.Account, error) {
	// user_id を text にして受けるのは、pgx の uuid 型を経由せずに
	// ドメインのコンストラクタへ通すため。表記の揺れ（大文字など）は
	// NewUserID が正規化する。
	var (
		storedProvider string
		storedSubject  string
		storedUserID   string
	)
	err := r.pool.QueryRow(ctx, `
		SELECT provider, subject, user_id::text FROM accounts
		WHERE provider = $1 AND subject = $2`,
		provider.String(), subject,
	).Scan(&storedProvider, &storedSubject, &storedUserID)

	if errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("%w: %s/%s", account.ErrAccountNotFound, provider, subject)
	}
	if err != nil {
		return nil, wrapUnavailable(err, "アカウントを読めない")
	}

	// 保存されている値も必ずコンストラクタを通す。素通しにすると、
	// 手で流した1行（docs/deploy.md の名寄せ）の綴り違いがそのまま
	// 生きたアカウントになる。
	p, err := account.NewProvider(storedProvider)
	if err != nil {
		return nil, fmt.Errorf("保存されたプロバイダが不正: %w", err)
	}
	uid, err := account.NewUserID(storedUserID)
	if err != nil {
		return nil, fmt.Errorf("保存された利用者の識別子が不正: %w", err)
	}
	a, err := account.NewAccount(p, storedSubject, uid)
	if err != nil {
		return nil, fmt.Errorf("保存されたアカウントが不正: %w", err)
	}
	return a, nil
}

// Create はアカウントを作る。
//
// ON CONFLICT DO NOTHING にせず一意制約違反を受けるのは、「既にあった」を
// 呼び出し側へ返すため。黙って無視すると、作ったつもりの UserID と
// 実際に入っている UserID が違う状態で先へ進む。
func (r *AccountRepository) Create(ctx context.Context, a *account.Account) error {
	if a == nil {
		return fmt.Errorf("アカウントが nil である")
	}

	_, err := r.pool.Exec(ctx, `
		INSERT INTO accounts (provider, subject, user_id) VALUES ($1, $2, $3)`,
		a.Provider().String(), a.Subject(), a.UserID().String())

	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == pgerrcode.UniqueViolation {
		return fmt.Errorf("%w: %s/%s",
			account.ErrAccountAlreadyExists, a.Provider(), a.Subject())
	}
	if err != nil {
		return wrapUnavailable(err, "アカウントを保存できない")
	}
	return nil
}

var (
	_ account.AccountReader = (*AccountRepository)(nil)
	_ account.AccountWriter = (*AccountRepository)(nil)
)
