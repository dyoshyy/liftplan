package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/dyoshyy/liftplan/internal/domain/account"
)

// SessionRepository はセッションの Postgres 実装。
type SessionRepository struct {
	pool *pgxpool.Pool
}

func NewSessionRepository(pool *pgxpool.Pool) *SessionRepository {
	return &SessionRepository{pool: pool}
}

// Find は now の時点で有効なセッションを返す。
//
// SQL に `expires_at > $2` と書かないのは、期限の規則を2箇所に置かない
// ため。SQL 側にも書くと、境界（期限ちょうど）の扱いを片方だけ直したときに
// インメモリ実装と答えが違う状態が緑のまま残る。判定は Session.IsExpired
// の1箇所だけにする。
//
// 引く行は主キーで1件なので、期限切れを SQL で落としても速さは変わらない。
func (r *SessionRepository) Find(
	ctx context.Context, hash account.TokenHash, now time.Time,
) (*account.Session, error) {
	var (
		storedUserID    string
		storedExpiresAt time.Time
	)
	err := r.pool.QueryRow(ctx, `
		SELECT user_id::text, expires_at FROM sessions WHERE token_hash = $1`,
		hash.String(),
	).Scan(&storedUserID, &storedExpiresAt)

	if errors.Is(err, pgx.ErrNoRows) {
		return nil, account.ErrSessionNotFound
	}
	if err != nil {
		return nil, wrapUnavailable(err, "セッションを読めない")
	}

	uid, err := account.NewUserID(storedUserID)
	if err != nil {
		return nil, fmt.Errorf("保存された利用者の識別子が不正: %w", err)
	}
	s, err := account.NewSession(hash, uid, storedExpiresAt)
	if err != nil {
		return nil, fmt.Errorf("保存されたセッションが不正: %w", err)
	}

	if s.IsExpired(now) {
		// 「知らないトークン」と同じエラーを返す。区別して返すと、
		// 当てずっぽうのトークンに「それは存在する」と答えることになる。
		return nil, account.ErrSessionNotFound
	}
	return s, nil
}

// Create はセッションを保存する。
//
// 入るのはトークンのハッシュだけ。トークンそのものは Session が
// 持っていないので、ここから漏らしようがない。
func (r *SessionRepository) Create(ctx context.Context, s *account.Session) error {
	if s == nil {
		return fmt.Errorf("セッションが nil である")
	}

	if _, err := r.pool.Exec(ctx, `
		INSERT INTO sessions (token_hash, user_id, expires_at) VALUES ($1, $2, $3)`,
		s.TokenHash().String(), s.UserID().String(), s.ExpiresAt()); err != nil {
		return wrapUnavailable(err, "セッションを保存できない")
	}
	return nil
}

// Delete はセッションを消す（ログアウト）。無いハッシュでも成功として扱う。
func (r *SessionRepository) Delete(ctx context.Context, hash account.TokenHash) error {
	if _, err := r.pool.Exec(ctx,
		"DELETE FROM sessions WHERE token_hash = $1", hash.String()); err != nil {
		return wrapUnavailable(err, "セッションを消せない")
	}
	return nil
}

var (
	_ account.SessionReader = (*SessionRepository)(nil)
	_ account.SessionWriter = (*SessionRepository)(nil)
)
