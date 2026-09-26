package memory

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/dyoshyy/liftplan/internal/domain/account"
)

// accountKey は (provider, subject) の組。map の鍵にできる形。
//
// 文字列を連結した鍵にしないのは、区切り文字を含む subject で
// 別の組と同じ鍵になりうるため。構造体なら連結の事故が起きない。
type accountKey struct {
	provider account.Provider
	subject  string
}

// AccountRepository はアカウントを (provider, subject) キーで保持する。
// 鍵が重複しないことが、Postgres 側の一意制約に対応する。
type AccountRepository struct {
	mu    sync.RWMutex
	byKey map[accountKey]*account.Account
}

func NewAccountRepository() *AccountRepository {
	return &AccountRepository{byKey: map[accountKey]*account.Account{}}
}

// Find は (provider, subject) のアカウントを返す。
func (r *AccountRepository) Find(
	_ context.Context, provider account.Provider, subject string,
) (*account.Account, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	a, ok := r.byKey[accountKey{provider: provider, subject: subject}]
	if !ok {
		return nil, fmt.Errorf("%w: %s/%s", account.ErrAccountNotFound, provider, subject)
	}
	return a, nil
}

// FindUserByEmail はそのメールアドレスを持つ利用者を返す。
//
// 索引は持たず、毎回なめる。行数は利用者の数しかなく、引くのも
// ログインのときだけ。Postgres 側で索引を付けなかったのと同じ理由。
func (r *AccountRepository) FindUserByEmail(
	_ context.Context, email account.Email,
) (account.UserID, error) {
	// 空では必ず見つからない。これが無いと、アドレスを持たない行が
	// 空のアドレスに当たり、その利用者が全員同一人物になる。
	if email.IsZero() {
		return account.UserID{}, fmt.Errorf(
			"%w: メールアドレスが空である", account.ErrAccountNotFound)
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	// 見つけた利用者が1種類かを見る。アカウントの件数ではない。同じ人が
	// GitHub と Google の両方で入れば同じアドレスの行が2つできるが、
	// それは正常な形で、曖昧ではない。
	var found account.UserID
	for _, a := range r.byKey {
		if a.Email() != email {
			continue
		}
		if found != (account.UserID{}) && a.UserID() != found {
			// どちらかを選ぶと、map の反復順次第で他人の記録に結びつく。
			return account.UserID{}, fmt.Errorf("%w: %s", account.ErrAmbiguousEmail, email)
		}
		found = a.UserID()
	}
	if found == (account.UserID{}) {
		return account.UserID{}, fmt.Errorf("%w: %s", account.ErrAccountNotFound, email)
	}
	return found, nil
}

// FindByUser はその利用者のアカウントをプロバイダ名の順に返す。
//
// map の反復順は毎回変わるので、並べ直してから返す。Postgres 側の
// ORDER BY と同じ並び。
func (r *AccountRepository) FindByUser(
	_ context.Context, userID account.UserID,
) ([]*account.Account, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	out := []*account.Account{}
	for _, a := range r.byKey {
		if a.UserID() == userID {
			out = append(out, a)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Provider() != out[j].Provider() {
			return out[i].Provider().String() < out[j].Provider().String()
		}
		return out[i].Subject() < out[j].Subject()
	})
	return out, nil
}

// Create はアカウントを作る。既にあれば ErrAccountAlreadyExists を返す。
//
// 上書きしないのは、上書きすると同じ人の UserID が入れ替わり、
// これまでの記録が見えなくなるため。
func (r *AccountRepository) Create(_ context.Context, a *account.Account) error {
	if a == nil {
		return fmt.Errorf("アカウントが nil である")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	key := accountKey{provider: a.Provider(), subject: a.Subject()}
	if _, ok := r.byKey[key]; ok {
		return fmt.Errorf("%w: %s/%s",
			account.ErrAccountAlreadyExists, a.Provider(), a.Subject())
	}
	r.byKey[key] = a
	return nil
}

// UpdateEmail はアカウントのアドレスだけを書き直す。
//
// 利用者は元の行から持ち越す。受け取らないのは、書き換えられる口を
// そもそも作らないため。
func (r *AccountRepository) UpdateEmail(
	_ context.Context, provider account.Provider, subject string, email account.Email,
) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	key := accountKey{provider: provider, subject: subject}
	a, ok := r.byKey[key]
	if !ok {
		return fmt.Errorf("%w: %s/%s", account.ErrAccountNotFound, provider, subject)
	}
	updated, err := account.NewAccount(a.Provider(), a.Subject(), a.UserID(), email)
	if err != nil {
		return fmt.Errorf("アカウントを組み直せない: %w", err)
	}
	r.byKey[key] = updated
	return nil
}

// SessionRepository はセッションをトークンのハッシュをキーに保持する。
type SessionRepository struct {
	mu     sync.RWMutex
	byHash map[account.TokenHash]*account.Session
}

func NewSessionRepository() *SessionRepository {
	return &SessionRepository{byHash: map[account.TokenHash]*account.Session{}}
}

// Find は now の時点で有効なセッションを返す。
//
// 期限の判定は Session.IsExpired に任せる。ここで now.After(...) と
// 書き直すと、規則が Postgres 実装と合わせて3箇所になる。
func (r *SessionRepository) Find(
	_ context.Context, hash account.TokenHash, now time.Time,
) (*account.Session, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	s, ok := r.byHash[hash]
	if !ok || s.IsExpired(now) {
		return nil, account.ErrSessionNotFound
	}
	return s, nil
}

// Create はセッションを保存する。
func (r *SessionRepository) Create(_ context.Context, s *account.Session) error {
	if s == nil {
		return fmt.Errorf("セッションが nil である")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.byHash[s.TokenHash()] = s
	return nil
}

// Delete はセッションを消す。無いハッシュでも成功として扱う。
func (r *SessionRepository) Delete(_ context.Context, hash account.TokenHash) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.byHash, hash)
	return nil
}

var (
	_ account.AccountReader = (*AccountRepository)(nil)
	_ account.AccountWriter = (*AccountRepository)(nil)
	_ account.SessionReader = (*SessionRepository)(nil)
	_ account.SessionWriter = (*SessionRepository)(nil)
)
