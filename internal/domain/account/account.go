package account

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

var (
	// ErrInvalidAccount はアカウントとして成立しない値を受け取ったことを表す。
	ErrInvalidAccount = errors.New("アカウントの内容が不正である")
	// ErrUnknownProvider は対応していない認証プロバイダを表す。
	ErrUnknownProvider = errors.New("知らない認証プロバイダである")
	// ErrAccountNotFound はその (provider, subject) のアカウントが無いことを表す。
	ErrAccountNotFound = errors.New("アカウントが見つからない")
	// ErrAccountAlreadyExists は同じ (provider, subject) が既にあることを表す。
	//
	// 黙って上書きしたり2件目を作ったりせず、作った側に返す。同時に2回
	// コールバックが来たときに同じ人のアカウントが2つできると、記録が
	// 2人分に割れて、本人からは「昨日までの記録が消えた」ように見える。
	ErrAccountAlreadyExists = errors.New("そのアカウントは既に存在する")
	// ErrAmbiguousEmail は同じメールアドレスに複数の利用者がぶら下がっている
	// ことを表す。
	//
	// 黙ってどちらかを選ぶと、選び方（保存順や索引の走査順）次第で他人の
	// 記録に結びつく。どちらか分からないことを呼び出し側へ返し、結ばない
	// 判断をさせる。
	ErrAmbiguousEmail = errors.New("そのメールアドレスに複数の利用者がいる")
)

// Provider はログインに使った認証プロバイダ。
//
// 定義型にしないのは UserID と同じ理由で、Provider("gihtub") と書けて
// しまうため。綴りを1文字間違えたものは別のプロバイダとして通り、既存の
// アカウントに当たらないので、初回ログイン扱いで新しい UserID ができる。
// 構造体で包めば、下の2つ以外は存在しない。
type Provider struct {
	v string
}

// プロバイダの名前。DB の provider 列に入る値でもある。
const (
	providerGitHubText = "github"
	providerGoogleText = "google"
)

// GitHub は GitHub でログインしたことを表す Provider を返す。
//
// var で公開すると誰でも書き換えられる。
func GitHub() Provider { return Provider{v: providerGitHubText} }

// Google は Google でログインしたことを表す Provider を返す。
func Google() Provider { return Provider{v: providerGoogleText} }

// NewProvider は文字列から Provider を作る。DB から読み戻す口でもある。
//
// 正規化（空白の除去・小文字化）は検証の前に行う。後にすると、検証を
// 通った値が正規化で別物になる。
func NewProvider(s string) (Provider, error) {
	v := strings.ToLower(strings.TrimSpace(s))
	switch v {
	case providerGitHubText, providerGoogleText:
		return Provider{v: v}, nil
	default:
		return Provider{}, fmt.Errorf("%w: %q", ErrUnknownProvider, s)
	}
}

// String はプロバイダの名前を返す。ゼロ値では空文字になる。
func (p Provider) String() string { return p.v }

// Email は確認済みのメールアドレス。
//
// 定義型にしないのは Provider と同じ理由で、Email("Foo@Example.com") と
// 書けてコンストラクタを素通りするため。素通りした値は正規化されず、
// 同じ人のアドレスが大文字と小文字で別物になる。
//
// ゼロ値（空）が正当な値。プロバイダがアドレスを返さないことも、
// 返しても未確認で捨てることもある。空を不正にすると、その人は
// ログインできなくなる。
type Email struct {
	v string
}

// NewEmail は文字列から Email を作る。
//
// **確認済みのアドレスだけを渡すこと。**確認したかどうかはプロバイダの
// 応答（GitHub の verified、Google の email_verified）にしか無く、ここでは
// 判定できない。未確認のものを渡すと、他人が自分のアドレスとして登録した
// だけで、そのアドレスの持ち主の記録に入れてしまう。
//
// 正規化（前後の空白の除去・小文字化）はここだけで行う。呼び出し側や
// リポジトリでもう一度かけると、規則が増えたときに揃わなくなる。
//
// 形式の検査はしない。エラーを返さないのはそのため。「@ が入っているか」は
// 確認済みであることの代わりにならず、通すか落とすかの判断を増やすだけ。
// プロバイダが確認したものをそのまま持つ。
func NewEmail(s string) Email {
	return Email{v: strings.ToLower(strings.TrimSpace(s))}
}

// String はメールアドレスを返す。持っていなければ空文字になる。
func (e Email) String() string { return e.v }

// IsZero はメールアドレスを持っていないことを返す。
func (e Email) IsZero() bool { return e.v == "" }

// Account は「そのプロバイダにおけるこの人」と UserID の対応。
//
// subject はプロバイダ側の不変な識別子（GitHub の数値ID、Google の sub）。
// メールアドレスやユーザー名で突き合わせないのは、どちらも本人が変更でき、
// 変えた瞬間に別人になる（＝これまでの記録が見えなくなる）ため。
//
// 確認済みのメールアドレスを併せて持つ。同一性を決めるのは依然として
// (provider, subject) で、アドレスは置いてあるだけ。GitHub と Google の
// アカウントを同じ人として結ぶかどうかを判断するのはこの型ではない。
//
// 0008 の時点では「メールで寄せると差し替えられたときに他人の記録へ
// 入れてしまう」として持たないことにしていた。確認済みのものに限り、
// かつ結ぶ判断を別に置くなら成り立つので、置き場だけを先に作る。
type Account struct {
	provider Provider
	subject  string
	userID   UserID
	email    Email
}

// NewAccount はアカウントを作る。
//
// email は確認済みのものか、空であること（NewEmail の doc を見ること）。
// 空は正当な値なので、ここでは弾かない。
func NewAccount(
	provider Provider, subject string, userID UserID, email Email,
) (*Account, error) {
	if provider == (Provider{}) {
		return nil, fmt.Errorf("%w: プロバイダが無い", ErrInvalidAccount)
	}
	// 空白の除去は検証の前。後にすると " " が非空として通り、
	// 空の subject を持つアカウントができる。
	s := strings.TrimSpace(subject)
	if s == "" {
		return nil, fmt.Errorf("%w: プロバイダ側の識別子が無い", ErrInvalidAccount)
	}
	// 小文字化はしない。Google の sub は大小を区別する不透明な文字列で、
	// 揃えると別人の識別子と衝突しうる。
	if userID == (UserID{}) {
		return nil, fmt.Errorf("%w: 利用者の識別子が無い", ErrInvalidAccount)
	}
	return &Account{provider: provider, subject: s, userID: userID, email: email}, nil
}

// Provider はログインに使ったプロバイダを返す。
func (a *Account) Provider() Provider { return a.provider }

// Subject はプロバイダ側の不変な識別子を返す。
func (a *Account) Subject() string { return a.subject }

// UserID はこのアカウントが指す利用者を返す。
func (a *Account) UserID() UserID { return a.userID }

// Email は確認済みのメールアドレスを返す。持っていなければゼロ値。
func (a *Account) Email() Email { return a.email }

// AccountReader はアカウントの読み出し口。
//
// Reader ではなく AccountReader なのは、このパッケージにアカウントと
// セッションの2つが同居するため（理由は doc.go）。
type AccountReader interface {
	// Find は (provider, subject) のアカウントを返す。
	// 無ければ ErrAccountNotFound を返す。
	Find(ctx context.Context, provider Provider, subject string) (*Account, error)

	// FindUserByEmail はそのメールアドレスを持つ利用者を返す。
	//
	// 該当が無ければ ErrAccountNotFound。空のアドレスでは**必ず**
	// 見つからない。空で全件に当たると、アドレスを持たない利用者が
	// 全員同一人物になる。
	//
	// 同じアドレスに異なる UserID が2つ以上ぶら下がっていたら
	// ErrAmbiguousEmail を返す。同じ UserID の行が複数あるのは正常
	// （同じ人が GitHub と Google の両方で入った形）なので、
	// 行数ではなく UserID の種類を数える。
	FindUserByEmail(ctx context.Context, email Email) (UserID, error)

	// FindByUser はその利用者のアカウントをプロバイダ名の順に返す。
	//
	// 無ければ空を返し、エラーにはしない。アカウントを通らずに入る
	// 利用者（開発用のセッション）がいるので、無いことは正常な形。
	FindByUser(ctx context.Context, userID UserID) ([]*Account, error)
}

// AccountWriter はアカウントの書き込み口。
type AccountWriter interface {
	// Create はアカウントを作る。同じ (provider, subject) が既にあれば
	// ErrAccountAlreadyExists を返す。既存の行を上書きしてはならない。
	Create(ctx context.Context, a *Account) error

	// UpdateEmail は (provider, subject) のアカウントのアドレスだけを
	// 書き直す。利用者は変えない。無ければ ErrAccountNotFound を返す。
	UpdateEmail(ctx context.Context, provider Provider, subject string, email Email) error
}
