package usecase

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/dyoshyy/liftplan/internal/domain/account"
	"github.com/dyoshyy/liftplan/internal/domain/training/exercise"
	"github.com/dyoshyy/liftplan/internal/domain/training/program"
	"github.com/dyoshyy/liftplan/internal/domain/training/seed"
)

// SignIn は初回・2回目以降のログインを受け入れるユースケース。
//
// プロバイダ側の identity（provider と subject）を受け取り、端末に渡す
// セッショントークンを返す。プロバイダとの通信（コード交換・identity の
// 取得）はここには無い。あれはインフラの仕事で、ここは「その人を受け入れる
// と何が起きるか」だけを持つ。
//
// **マイグレーション 0007 が既存の記録を寄せた利用者を特別扱いしない。**
// 「最初にログインした人が既存の記録を引き継ぐ」にはしていない。そうすると、
// デプロイ直後に見知らぬ人が先にログインしただけで、これまでの記録を全部
// 持っていかれる。既存の記録を本人に結ぶのは、初回ログインの前に1回だけ
// 手で流す INSERT の仕事（設計書 2026-09-21-oauth-multi-user-design.md）。
// ここから見れば、本人も他の誰も、ただの初回ログインである。
type SignIn struct {
	accounts      account.AccountReader
	accountWriter account.AccountWriter
	sessions      account.SessionWriter
	programs      program.Reader
	programWriter program.Writer
	exercises     exercise.Reader
}

func NewSignIn(
	accounts account.AccountReader,
	accountWriter account.AccountWriter,
	sessions account.SessionWriter,
	programs program.Reader,
	programWriter program.Writer,
	exercises exercise.Reader,
) *SignIn {
	return &SignIn{
		accounts:      accounts,
		accountWriter: accountWriter,
		sessions:      sessions,
		programs:      programs,
		programWriter: programWriter,
		exercises:     exercises,
	}
}

// Execute はその identity を受け入れ、セッショントークンを返す。
//
// now を引数で受けるのは、セッションの期限を内部の time.Now() で決めると
// 期限のテストが書けなくなるため（90日待つことになる）。現在時刻を作るのは
// プレゼンテーション層の仕事で、これは httpapi/read.go の日付と同じ扱い。
//
// 順序が効く。セッションは**最後**に発行する。途中で失敗したときに
// 「ログインはできているのに使えない」利用者を作らないため。
func (u *SignIn) Execute(
	ctx context.Context,
	identity account.Identity,
	now time.Time,
) (_ account.SessionToken, err error) {
	// 出口で1度だけ翻訳する。return ごとに書くと、経路が増えたときに
	// 包み忘れた1本だけが 500 で返る。
	defer func() { err = classify(err) }()

	if err := ctx.Err(); err != nil {
		return account.SessionToken{}, fmt.Errorf("ログインが中断された: %w", err)
	}

	a, err := u.findOrCreateAccount(ctx, identity)
	if err != nil {
		return account.SessionToken{}, err
	}

	if err := u.seedProgramIfMissing(ctx, a.UserID()); err != nil {
		return account.SessionToken{}, err
	}

	token, s, err := account.IssueSession(a.UserID(), now)
	if err != nil {
		return account.SessionToken{}, fmt.Errorf("セッションを発行できない: %w", err)
	}
	if err := u.sessions.Create(ctx, s); err != nil {
		return account.SessionToken{}, fmt.Errorf("セッションを保存できない: %w", err)
	}
	return token, nil
}

// resolveUser は、このアカウントを誰のものにするかを決める。
//
// 確認済みアドレスが同じ利用者が既にいれば、その人に結ぶ。これが
// 「GitHub で入った人が Google を押しても同じ記録が見える」の全部。
//
// # 呼ぶのはアカウントを作るときだけ
//
// 既にあるアカウントの利用者は決して書き換えない。書き換える経路が
// あると、**認可先のメールアドレスを他人のものに変えて入り直すだけで、
// 他人の記録に入れる。**結ぶのは作るときの1回に限る。
//
// # 結べないときは結ばない。失敗にはしない
//
// アドレスが無い・一致する人がいない・同じアドレスに複数の利用者がいる。
// どれも「分からない」であって「壊れている」ではないので、新しい利用者を
// 採番して先へ進む。**複数いるときに片方を選ばない**のが要点で、
// 選び方次第で他人の記録に結びついてしまう。
//
// # 引きが壊れたときだけ失敗させる
//
// 保存先に届かなかったときに「見つからなかった」と同じ扱いにすると、
// DB が一瞬応えなかっただけで**本人の記録から切り離された利用者が
// 黙って生まれる**。あとから直すには、どの行が誤ってできたかを人が
// 突き合わせるしかない。ここは素直に失敗させ、押し直してもらう。
func (u *SignIn) resolveUser(ctx context.Context, email account.Email) (account.UserID, error) {
	switch linked, err := u.accounts.FindUserByEmail(ctx, email); {
	case err == nil:
		return linked, nil
	case errors.Is(err, account.ErrAccountNotFound),
		errors.Is(err, account.ErrAmbiguousEmail):
		// 結べない。新しい利用者として扱う。
	default:
		return account.UserID{}, fmt.Errorf("アドレスから利用者を引けない: %w", err)
	}

	userID, err := account.NewRandomUserID()
	if err != nil {
		return account.UserID{}, fmt.Errorf("利用者の識別子を採番できない: %w", err)
	}
	return userID, nil
}

// findOrCreateAccount は (provider, subject) のアカウントを返し、無ければ作る。
//
// 作ろうとして「既にある」が返ったら、引き直して続行する。同じ人の
// コールバックが同時に2回来ると、両方が「無い」を見て両方が作りにいき、
// 一意制約により片方は必ず失敗する。そこでエラーを返すと、**片方の
// リクエストだけが失敗する不安定な経路**になる。本人から見れば
// 「たまにログインに失敗する」で、再現もしない。
//
// 引き直しは1回だけ。見つからなければエラーにする。ループにすると、
// リポジトリが「無い」と「既にある」を交互に返す壊れ方をしたときに
// 回り続ける。
func (u *SignIn) findOrCreateAccount(
	ctx context.Context, identity account.Identity,
) (*account.Account, error) {
	a, err := u.accounts.Find(ctx, identity.Provider(), identity.Subject())
	switch {
	case err == nil:
		return a, nil
	case !errors.Is(err, account.ErrAccountNotFound):
		return nil, fmt.Errorf("アカウントの取得に失敗: %w", err)
	}

	// ここから初回。**この1回だけ**、既にいる利用者に結べるかを見る。
	userID, err := u.resolveUser(ctx, identity.Email())
	if err != nil {
		return nil, err
	}
	created, err := account.NewAccount(
		identity.Provider(), identity.Subject(), userID, identity.Email())
	if err != nil {
		return nil, fmt.Errorf("アカウントを組めない: %w", err)
	}

	switch err := u.accountWriter.Create(ctx, created); {
	case err == nil:
		return created, nil
	case !errors.Is(err, account.ErrAccountAlreadyExists):
		return nil, fmt.Errorf("アカウントの作成に失敗: %w", err)
	}

	// 先を越された。相手が作ったほうを使う。自分が採番した UserID で
	// 続けると、同じ人の記録が2人分に割れる。
	a, err = u.accounts.Find(ctx, identity.Provider(), identity.Subject())
	if err != nil {
		return nil, fmt.Errorf("先に作られたアカウントを引けない: %w", err)
	}
	return a, nil
}

// seedProgramIfMissing はその利用者にプログラムが無ければ初期値を入れる。
//
// **初回かどうかを「アカウントを作れたか」では判断しない。**そうすると
// 順序に依存する。アカウントを作ってからプログラムの保存に失敗すると、
// 以後その利用者は永久に「2回目」になり、プログラムを持たないまま残る。
// 未設定かどうかを直接見れば、途中で落ちても次のログインでやり直せる。
//
// 逆に、既にあるなら触らない。毎回入れ直すと、本人が変えた設定が
// 再ログインのたびに初期値へ戻る。しかも何度でも起きる。
func (u *SignIn) seedProgramIfMissing(ctx context.Context, userID account.UserID) error {
	switch _, err := u.programs.Get(ctx, userID); {
	case err == nil:
		return nil
	case !errors.Is(err, program.ErrProgramNotConfigured):
		return fmt.Errorf("プログラムの確認に失敗: %w", err)
	}

	pool, err := u.exercises.FindAll(ctx)
	if err != nil {
		return fmt.Errorf("種目の取得に失敗: %w", err)
	}
	prog, err := seed.DefaultProgram(pool)
	if err != nil {
		return fmt.Errorf("初期プログラムを組めない: %w", err)
	}
	if err := u.programWriter.Save(ctx, userID, prog); err != nil {
		return fmt.Errorf("初期プログラムを保存できない: %w", err)
	}
	return nil
}
