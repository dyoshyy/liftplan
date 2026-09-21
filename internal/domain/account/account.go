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
// var で公開すると誰でも書き換えられる（DefaultUserID と同じ理由）。
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

// Account は「そのプロバイダにおけるこの人」と UserID の対応。
//
// subject はプロバイダ側の不変な識別子（GitHub の数値ID、Google の sub）。
// メールアドレスやユーザー名で突き合わせないのは、どちらも本人が変更でき、
// 変えた瞬間に別人になる（＝これまでの記録が見えなくなる）ため。
//
// 同じ人が GitHub と Google の両方でログインしたら別人になる。名寄せは
// しない。メールで寄せると、プロバイダ側でメールを差し替えられたときに
// 他人の記録へ入れてしまう。本人に「どちらで入るか」を選ばせるのも、
// 消したかった判断を増やすだけ。実際に困ってから作る。
type Account struct {
	provider Provider
	subject  string
	userID   UserID
}

// NewAccount はアカウントを作る。
func NewAccount(provider Provider, subject string, userID UserID) (*Account, error) {
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
	return &Account{provider: provider, subject: s, userID: userID}, nil
}

// Provider はログインに使ったプロバイダを返す。
func (a *Account) Provider() Provider { return a.provider }

// Subject はプロバイダ側の不変な識別子を返す。
func (a *Account) Subject() string { return a.subject }

// UserID はこのアカウントが指す利用者を返す。
func (a *Account) UserID() UserID { return a.userID }

// AccountReader はアカウントの読み出し口。
//
// Reader ではなく AccountReader なのは、このパッケージにアカウントと
// セッションの2つが同居するため（理由は doc.go）。
type AccountReader interface {
	// Find は (provider, subject) のアカウントを返す。
	// 無ければ ErrAccountNotFound を返す。
	Find(ctx context.Context, provider Provider, subject string) (*Account, error)
}

// AccountWriter はアカウントの書き込み口。
type AccountWriter interface {
	// Create はアカウントを作る。同じ (provider, subject) が既にあれば
	// ErrAccountAlreadyExists を返す。既存の行を上書きしてはならない。
	Create(ctx context.Context, a *Account) error
}
