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
	//
	// email は coalesce で空文字に落とす。NULL（当時アドレスを取って
	// いなかった行）と、アドレスを持たないことは同じ意味。
	var (
		storedProvider string
		storedSubject  string
		storedUserID   string
		storedEmail    string
	)
	err := r.pool.QueryRow(ctx, `
		SELECT provider, subject, user_id::text, coalesce(email, '') FROM accounts
		WHERE provider = $1 AND subject = $2`,
		provider.String(), subject,
	).Scan(&storedProvider, &storedSubject, &storedUserID, &storedEmail)

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
	a, err := account.NewAccount(p, storedSubject, uid, account.NewEmail(storedEmail))
	if err != nil {
		return nil, fmt.Errorf("保存されたアカウントが不正: %w", err)
	}
	return a, nil
}

// FindUserByEmail はそのメールアドレスを持つ利用者を返す。
func (r *AccountRepository) FindUserByEmail(
	ctx context.Context, email account.Email,
) (account.UserID, error) {
	// 空では問い合わせない。NULL は = '' に当たらないので実際には
	// 何も返らないが、SQL の NULL の扱いに寄りかからない。列の入り方を
	// 変えた人（空文字で埋めるなど）がここを落とすと、アドレスを
	// 持たない利用者が全員同一人物になる。
	if email.IsZero() {
		return account.UserID{}, fmt.Errorf(
			"%w: メールアドレスが空である", account.ErrAccountNotFound)
	}

	// DISTINCT で利用者の種類を数える。行を数えると、同じ人が GitHub と
	// Google の両方で入っている正常な形が曖昧になり、結べるはずの2つが
	// 永久に結ばれない。
	//
	// LIMIT 2 なのは、2種類あることが分かれば十分だから。3人目以降を
	// 読んでも返す答えは変わらない。
	rows, err := r.pool.Query(ctx, `
		SELECT DISTINCT user_id::text FROM accounts WHERE email = $1 LIMIT 2`,
		email.String())
	if err != nil {
		return account.UserID{}, wrapUnavailable(err, "メールアドレスで引けない")
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return account.UserID{}, wrapUnavailable(err, "メールアドレスで引けない")
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return account.UserID{}, wrapUnavailable(err, "メールアドレスで引けない")
	}

	switch len(ids) {
	case 0:
		return account.UserID{}, fmt.Errorf("%w: %s", account.ErrAccountNotFound, email)
	case 1:
		uid, err := account.NewUserID(ids[0])
		if err != nil {
			return account.UserID{}, fmt.Errorf("保存された利用者の識別子が不正: %w", err)
		}
		return uid, nil
	default:
		// どちらかを選ぶと、選び方次第で他人の記録に結びつく。
		return account.UserID{}, fmt.Errorf("%w: %s", account.ErrAmbiguousEmail, email)
	}
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

	// NULLIF で空文字を NULL に落とす。空文字のまま入れると、アドレスを
	// 持たない行どうしが「同じアドレス」として並び、曖昧の判定に乗る。
	_, err := r.pool.Exec(ctx, `
		INSERT INTO accounts (provider, subject, user_id, email)
		VALUES ($1, $2, $3, NULLIF($4, ''))`,
		a.Provider().String(), a.Subject(), a.UserID().String(), a.Email().String())

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
