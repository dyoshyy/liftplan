package account

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"
)

var (
	// ErrInvalidSession はセッションとして成立しない値を受け取ったことを表す。
	ErrInvalidSession = errors.New("セッションの内容が不正である")
	// ErrInvalidSessionToken はセッショントークンとして使えない値を表す。
	ErrInvalidSessionToken = errors.New("セッショントークンが不正である")
	// ErrSessionNotFound は有効なセッションが無いことを表す。
	//
	// 「そのトークンを知らない」と「期限が切れている」を区別しない。
	// 区別して返すと、当てずっぽうのトークンに対して「それは存在するが
	// 期限切れだ」と答えることになる。呼び出し側がやることはどちらでも
	// 同じ（401 を返してログインし直させる）。
	ErrSessionNotFound = errors.New("有効なセッションが無い")
)

// SessionLifetime はセッションの有効期間。
//
// 90日。ジムで毎回ログインし直すアプリに価値は無い。トークンは端末の
// localStorage にあり、漏れたときの逃げ道はログアウト（行の削除）。
const SessionLifetime = 90 * 24 * time.Hour

// sessionTokenBytes は生成するトークンの乱数バイト数。
//
// 32バイト＝256ビット。総当たりで当てられる長さではない。
const sessionTokenBytes = 32

// SessionToken は端末だけが持つ秘密。
//
// **この値は保存しない。**保存するのは Hash() が返す SHA-256 のハッシュだけで、
// Session はこの型のフィールドを持たない。DB が漏れてもセッションは漏れない
// （ハッシュから元のトークンは作れない）。引くのもハッシュなので、
// 定数時間比較も要らない。
//
// 生成（crypto/rand）をドメインに置いたのは、トークンの長さと乱数源が
// 「セッションとは何か」の一部だから。インフラに置くと、実装ごとに
// 別の長さ・別の乱数源になりうるし、それが安全かはインフラの各実装を
// 読まないと分からなくなる。標準ライブラリしか使わないので、
// 「DBもHTTPも立てずに全機能をテストできる」は壊れない（D-118）。
type SessionToken struct {
	v string
}

// NewSessionToken は新しいトークンを作る。
//
// crypto/rand が失敗したらエラーを返す。math/rand へ落とさない。
// 落とすと、推測できるトークンを「発行できた」として返すことになる。
func NewSessionToken() (SessionToken, error) {
	b := make([]byte, sessionTokenBytes)
	if _, err := rand.Read(b); err != nil {
		return SessionToken{}, fmt.Errorf("%w: 乱数を取得できない: %w",
			ErrInvalidSessionToken, err)
	}
	// URL 安全な表記。フラグメント（#token=...）に載せるので、
	// パディングや + / が混じると受け側で壊れる。
	return SessionToken{v: base64.RawURLEncoding.EncodeToString(b)}, nil
}

// ParseSessionToken は端末から送られてきた文字列を SessionToken にする。
//
// 中身の形は検査しない（総当たりの失敗は Hash で引けないことで表れる）。
// 空だけを弾く。空を通すと「空文字のハッシュ」で引く経路ができる。
func ParseSessionToken(s string) (SessionToken, error) {
	v := strings.TrimSpace(s)
	if v == "" {
		return SessionToken{}, fmt.Errorf("%w: 空である", ErrInvalidSessionToken)
	}
	return SessionToken{v: v}, nil
}

// String はトークンの文字列を返す。端末へ渡すときだけ使う。
func (t SessionToken) String() string { return t.v }

// Hash はこのトークンの SHA-256 ハッシュを返す。
func (t SessionToken) Hash() TokenHash {
	sum := sha256.Sum256([]byte(t.v))
	return TokenHash{v: hex.EncodeToString(sum[:])}
}

// tokenHashLength は SHA-256 を16進で書いた長さ。
const tokenHashLength = sha256.Size * 2

// TokenHash はセッショントークンの SHA-256 ハッシュ。DB に入るのはこちら。
type TokenHash struct {
	v string
}

// NewTokenHash は16進文字列から TokenHash を作る。DB から読み戻す口。
//
// 正規化（空白の除去・小文字化）は検証の前。後にすると、検証を通った値が
// 正規化で別物になる。
func NewTokenHash(s string) (TokenHash, error) {
	v := strings.ToLower(strings.TrimSpace(s))
	if len(v) != tokenHashLength {
		return TokenHash{}, fmt.Errorf("%w: %q は SHA-256 の16進表記ではない",
			ErrInvalidSessionToken, s)
	}
	for i := range len(v) {
		if !isHexDigit(v[i]) {
			return TokenHash{}, fmt.Errorf("%w: %q は SHA-256 の16進表記ではない",
				ErrInvalidSessionToken, s)
		}
	}
	return TokenHash{v: v}, nil
}

// String は16進のハッシュを返す。ゼロ値では空文字になる。
func (h TokenHash) String() string { return h.v }

// Session はログイン中の端末1つ。
//
// トークンそのものは持たない（SessionToken の説明を見よ）。持たないので、
// この構造体をそのままログに出してもトークンは漏れない。
type Session struct {
	tokenHash TokenHash
	userID    UserID
	expiresAt time.Time
}

// IssueSession は新しいトークンとセッションを発行する。
//
// 現在時刻を引数で受けるのは、内部で time.Now() を呼ぶと期限のテストが
// 書けなくなるため（90日待つことになる）。
func IssueSession(userID UserID, now time.Time) (SessionToken, *Session, error) {
	token, err := NewSessionToken()
	if err != nil {
		return SessionToken{}, nil, err
	}
	s, err := NewSession(token.Hash(), userID, now.Add(SessionLifetime))
	if err != nil {
		return SessionToken{}, nil, err
	}
	return token, s, nil
}

// NewSession は保存済みのセッションを組み立て直す。
func NewSession(hash TokenHash, userID UserID, expiresAt time.Time) (*Session, error) {
	if hash == (TokenHash{}) {
		return nil, fmt.Errorf("%w: トークンのハッシュが無い", ErrInvalidSession)
	}
	if userID == (UserID{}) {
		return nil, fmt.Errorf("%w: 利用者の識別子が無い", ErrInvalidSession)
	}
	if expiresAt.IsZero() {
		return nil, fmt.Errorf("%w: 期限が無い", ErrInvalidSession)
	}
	// UTC への寄せは検証の前ではなく後でよい（ゼロ値かどうかは
	// タイムゾーンで変わらない）。寄せるのは、保存と読み戻しで
	// 場所が変わっても同じ時刻として比べられるようにするため。
	return &Session{tokenHash: hash, userID: userID, expiresAt: expiresAt.UTC()}, nil
}

// TokenHash はこのセッションのトークンハッシュを返す。
func (s *Session) TokenHash() TokenHash { return s.tokenHash }

// UserID はこのセッションの持ち主を返す。
func (s *Session) UserID() UserID { return s.userID }

// ExpiresAt は期限を返す。
func (s *Session) ExpiresAt() time.Time { return s.expiresAt }

// IsExpired は now の時点で期限切れかを返す。
//
// 期限ちょうどは**切れている**扱い。有効なのは now < expiresAt の間だけ。
// 境界をどちらに倒すかはどちらでもよいが、決めておかないと実装ごとに
// 違う答えになる。倒す先は「切れている」側にした。期限を1ナノ秒でも
// 過ぎたものが通るより、ちょうどのものが弾かれるほうが説明しやすい。
//
// 判定をドメインに置き、リポジトリは SQL の WHERE ではなくこの関数を
// 呼ぶ。SQL 側にも同じ規則を書くと、規則が2箇所になって片方だけ
// 直したときに実装ごとに答えが変わる。
func (s *Session) IsExpired(now time.Time) bool {
	return !now.Before(s.expiresAt)
}

// SessionReader はセッションの読み出し口。
type SessionReader interface {
	// Find は now の時点で有効なセッションを返す。
	// 無いか期限切れなら ErrSessionNotFound を返す。
	Find(ctx context.Context, hash TokenHash, now time.Time) (*Session, error)
}

// SessionWriter はセッションの書き込み口。
type SessionWriter interface {
	// Create はセッションを保存する。
	Create(ctx context.Context, s *Session) error
	// Delete はセッションを消す（ログアウト）。
	// 無いハッシュを渡されても成功として扱う。ログアウトの目的は
	// 「消えていること」で、既に消えているなら達成されている。
	Delete(ctx context.Context, hash TokenHash) error
}
