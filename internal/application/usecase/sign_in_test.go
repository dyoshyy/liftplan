package usecase_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/dyoshyy/liftplan/internal/application/usecase"
	"github.com/dyoshyy/liftplan/internal/domain/account"
	"github.com/dyoshyy/liftplan/internal/domain/training/program"
	"github.com/dyoshyy/liftplan/internal/domain/training/seed"
)

// signInFixture は SignIn とその保存先をまとめて持つ。
//
// 検査したいのは「何が保存されたか」なので、ユースケースだけでなく
// 保存先そのものを持っておく。
//
// 保存先は internal/infrastructure/memory ではなくこのファイルで組む。
// application 層のテストが infrastructure を import すると、
// internal/architecture_test.go の依存方向の検査に落ちる（テストファイルが
// 対象外になるのは「同じ深さの別の層」だけで、外側の層は対象外にならない）。
type signInFixture struct {
	signIn   *usecase.SignIn
	accounts *signInAccounts
	sessions *signInSessions
	programs *signInPrograms
}

func newSignInFixture(t *testing.T) *signInFixture {
	t.Helper()
	return newSignInFixtureWith(t, nil)
}

// newSignInFixtureWith はプログラムの保存口を差し替えられる版。
// nil ならインメモリの保存先をそのまま使う。
func newSignInFixtureWith(t *testing.T, programWriter program.Writer) *signInFixture {
	t.Helper()

	pool, err := seed.Exercises()
	if err != nil {
		t.Fatalf("シードが不正: %v", err)
	}

	accounts := newSignInAccounts()
	sessions := newSignInSessions()
	programs := newSignInPrograms()

	var writer program.Writer = programs
	if programWriter != nil {
		writer = programWriter
	}

	return &signInFixture{
		signIn: usecase.NewSignIn(
			accounts, accounts, sessions, programs, writer,
			&fakeExercises{all: pool}),
		accounts: accounts,
		sessions: sessions,
		programs: programs,
	}
}

func githubSubject() (account.Provider, string) {
	return account.GitHub(), "12345678"
}

var signInNow = time.Date(2026, 9, 21, 10, 0, 0, 0, time.UTC)

// 初回ログインで、アカウント・初期プログラム・セッションが揃うこと。
//
// どれか1つでも欠けると、ログインはできたのに使えない状態になる。
// 特にプログラムが無いと、画面は「設定してください」しか出せない。
func TestSignIn_FirstTimeCreatesEverything(t *testing.T) {
	f := newSignInFixture(t)
	provider, subject := githubSubject()

	token, err := f.signIn.Execute(context.Background(), provider, subject, signInNow)
	if err != nil {
		t.Fatalf("初回ログインに失敗: %v", err)
	}
	if token.String() == "" {
		t.Fatal("トークンが空である")
	}

	a, err := f.accounts.Find(context.Background(), provider, subject)
	if err != nil {
		t.Fatalf("アカウントが作られていない: %v", err)
	}
	// 既定ユーザーに寄せてはいけない。寄せると、デプロイ直後に見知らぬ人が
	// 先にログインしただけで、これまでの記録を全部持っていかれる。
	// 本人のアカウントを既定ユーザーに結ぶのは、初回ログインの前に手で流す
	// 1行の仕事で、このユースケースの仕事ではない。
	if a.UserID() == account.DefaultUserID() {
		t.Error("採番せず既定ユーザーを割り当てている")
	}

	// 初期プログラムが「その利用者の」下にあること。
	prog, err := f.programs.Get(context.Background(), a.UserID())
	if err != nil {
		t.Fatalf("初期プログラムが作られていない: %v", err)
	}
	if got := prog.Frequency().PerWeek(); got != seed.DefaultFrequencyPerWeek {
		t.Errorf("初期プログラムの頻度が %d。シードの %d のはず",
			got, seed.DefaultFrequencyPerWeek)
	}

	// セッションが1件。トークンで引けること（＝返したトークンが
	// 保存したセッションのものであること）。
	if got := f.sessions.created(); got != 1 {
		t.Errorf("セッションが %d 件。1件のはず", got)
	}
	s, err := f.sessions.find(token.Hash(), signInNow)
	if err != nil {
		t.Fatalf("返したトークンでセッションを引けない: %v", err)
	}
	if s.UserID() != a.UserID() {
		t.Errorf("セッションの持ち主が %q。アカウントの %q のはず",
			s.UserID(), a.UserID())
	}
	// 期限が now から始まっていること。past の now で作ると 90日後より
	// 手前になるので、ここが合わなければ now を使っていない。
	if want := signInNow.Add(account.SessionLifetime); !s.ExpiresAt().Equal(want) {
		t.Errorf("期限が %v。渡した時刻から %v 後の %v のはず",
			s.ExpiresAt(), account.SessionLifetime, want)
	}
}

// 2回目のログインでアカウントが増えないこと。同じ UserID のセッションが
// もう1件増え、トークンは毎回違うこと。
//
// アカウントが2つできると、同じ人の記録が2人分に割れて、本人からは
// 「昨日までの記録が消えた」ように見える。
func TestSignIn_SecondTimeReusesTheAccount(t *testing.T) {
	f := newSignInFixture(t)
	provider, subject := githubSubject()

	first, err := f.signIn.Execute(context.Background(), provider, subject, signInNow)
	if err != nil {
		t.Fatalf("初回ログインに失敗: %v", err)
	}
	second, err := f.signIn.Execute(
		context.Background(), provider, subject, signInNow.Add(time.Hour))
	if err != nil {
		t.Fatalf("2回目のログインに失敗: %v", err)
	}

	if got := f.accounts.created(); got != 1 {
		t.Errorf("アカウントを %d 回作っている。1回のはず", got)
	}
	if first.String() == second.String() {
		t.Error("2回目に同じトークンが返っている。端末ごとに別のセッションにならない")
	}
	if got := f.sessions.created(); got != 2 {
		t.Errorf("セッションが %d 件。2件のはず（端末2台でログインしたのと同じ）", got)
	}

	// どちらのトークンでも同じ利用者として引けること。
	s1, err := f.sessions.find(first.Hash(), signInNow)
	if err != nil {
		t.Fatalf("1本目のトークンで引けない: %v", err)
	}
	s2, err := f.sessions.find(second.Hash(), signInNow)
	if err != nil {
		t.Fatalf("2本目のトークンで引けない: %v", err)
	}
	if s1.UserID() != s2.UserID() {
		t.Errorf("2回目で別の利用者になっている: %q → %q", s1.UserID(), s2.UserID())
	}
}

// 2回目のログインで、本人が変えた設定を初期プログラムで上書きしないこと。
//
// **これが一番こわい。**設定を変えたあと再ログインしただけで週3回に戻ったら、
// 本人からは「勝手に設定が消えた」ように見える。しかも何度でも起きる。
func TestSignIn_SecondTimeDoesNotOverwriteTheProgram(t *testing.T) {
	f := newSignInFixture(t)
	provider, subject := githubSubject()

	if _, err := f.signIn.Execute(
		context.Background(), provider, subject, signInNow); err != nil {
		t.Fatalf("初回ログインに失敗: %v", err)
	}
	a, err := f.accounts.Find(context.Background(), provider, subject)
	if err != nil {
		t.Fatalf("アカウントが作られていない: %v", err)
	}

	// 本人が頻度を変えた。シードの既定（3）とは違う値にする。
	changed := changeFrequency(t, f, a.UserID())

	if _, err := f.signIn.Execute(
		context.Background(), provider, subject, signInNow.Add(time.Hour)); err != nil {
		t.Fatalf("2回目のログインに失敗: %v", err)
	}

	prog, err := f.programs.Get(context.Background(), a.UserID())
	if err != nil {
		t.Fatalf("プログラムが消えている: %v", err)
	}
	if got := prog.Frequency().PerWeek(); got != changed {
		t.Errorf("再ログインで頻度が %d に戻った。本人が設定した %d のままのはず",
			got, changed)
	}
}

// changeFrequency は保存済みのプログラムの頻度を既定と違う値に変えて、
// その値を返す。
func changeFrequency(t *testing.T, f *signInFixture, userID account.UserID) int {
	t.Helper()

	const perWeek = 5 // シードの既定（3）と違う値なら何でもよい
	prog, err := f.programs.Get(context.Background(), userID)
	if err != nil {
		t.Fatalf("プログラムを取得できない: %v", err)
	}
	freq, err := program.NewFrequency(perWeek)
	if err != nil {
		t.Fatalf("頻度が不正: %v", err)
	}
	target, err := seed.DefaultWeeklyTarget(freq)
	if err != nil {
		t.Fatalf("週目標シードが不正: %v", err)
	}
	changed, err := program.NewProgram(
		freq, target, prog.SelectedExercises(), prog.DeclaredExercises(), "")
	if err != nil {
		t.Fatalf("プログラムが不正: %v", err)
	}
	if err := f.programs.Save(context.Background(), userID, changed); err != nil {
		t.Fatalf("プログラムを保存できない: %v", err)
	}
	return perWeek
}

// 作ろうとしたら既にあった場合、引き直して成功すること。
//
// 同じ人のコールバックが同時に2回来ると、両方が「無い」を見て両方が作る。
// (provider, subject) には一意制約があるので片方は必ず失敗する。そこで
// エラーを返すと、**片方のリクエストだけが失敗する不安定な経路**になる。
// 本人から見れば「たまにログインに失敗する」で、再現もしない。
func TestSignIn_RecoversWhenTheAccountAppearsFirst(t *testing.T) {
	f := newSignInFixture(t)
	provider, subject := githubSubject()

	// 先に割り込んだ側が作ったアカウント。こちらが Create する瞬間に
	// 見えるようになる。goroutine を使わずに同時実行の結果だけを作る。
	winner := mustUserID(t)
	f.accounts.insertOnNextCreate(t, provider, subject, winner)

	token, err := f.signIn.Execute(context.Background(), provider, subject, signInNow)
	if err != nil {
		t.Fatalf("既にアカウントがある場合にログインが失敗した: %v", err)
	}

	s, err := f.sessions.find(token.Hash(), signInNow)
	if err != nil {
		t.Fatalf("セッションが発行されていない: %v", err)
	}
	// 自分が採番した UserID ではなく、先にできていたほうを使うこと。
	// 自分の採番で続けると、同じ人の記録が2人分に割れる。
	if s.UserID() != winner {
		t.Errorf("セッションの持ち主が %q。先にできていた %q のはず",
			s.UserID(), winner)
	}
	// 先にできていたほうにプログラムが入ること。自分の採番のほうに
	// 入れると、本人が使う UserID の下は空のままになる。
	if _, err := f.programs.Get(context.Background(), winner); err != nil {
		t.Errorf("先にできていた利用者に初期プログラムが無い: %v", err)
	}
}

// 初期プログラムの保存に失敗したら、セッションを発行しないこと。
//
// 発行すると、プログラムを持たない利用者がログインした状態になる。
// 画面からは何も操作できない。
func TestSignIn_IssuesNoSessionWhenTheProgramCannotBeSaved(t *testing.T) {
	wantErr := errors.New("保存先が落ちている")
	f := newSignInFixtureWith(t, &failingProgramWriter{err: wantErr})
	provider, subject := githubSubject()

	_, err := f.signIn.Execute(context.Background(), provider, subject, signInNow)
	if err == nil {
		t.Fatal("プログラムを保存できないのに成功している")
	}
	// %w で包むこと。包まないと、呼び出し側が原因で分岐できない。
	if !errors.Is(err, wantErr) {
		t.Errorf("原因が連鎖に残っていない: %v", err)
	}
	if got := f.sessions.created(); got != 0 {
		t.Errorf("セッションを %d 件発行している。0件のはず", got)
	}
}

// --- テスト用の保存先 ---

// signInAccountKey は (provider, subject) の組。Postgres 側の一意制約に対応する。
type signInAccountKey struct {
	provider account.Provider
	subject  string
}

// signInAccounts はアカウントの保存先。Create の回数を数える。
//
// 「アカウントが増えない」は保存先の件数では見られない（同じ鍵なら
// 上書きでも1件になる）。作ろうとした回数そのものを見る。
type signInAccounts struct {
	mu        sync.Mutex
	byKey     map[signInAccountKey]*account.Account
	createCnt int
	// onCreate は Create の直前に1度だけ走る。同時実行で先を越された
	// 状況を、goroutine を使わずに作るための仕掛け。
	onCreate func()
}

func newSignInAccounts() *signInAccounts {
	return &signInAccounts{byKey: map[signInAccountKey]*account.Account{}}
}

func (r *signInAccounts) Find(
	_ context.Context, provider account.Provider, subject string,
) (*account.Account, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	a, ok := r.byKey[signInAccountKey{provider: provider, subject: subject}]
	if !ok {
		return nil, fmt.Errorf("%w: %s/%s", account.ErrAccountNotFound, provider, subject)
	}
	return a, nil
}

func (r *signInAccounts) Create(_ context.Context, a *account.Account) error {
	r.mu.Lock()
	r.createCnt++
	hook := r.onCreate
	r.onCreate = nil
	r.mu.Unlock()

	if hook != nil {
		hook()
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	key := signInAccountKey{provider: a.Provider(), subject: a.Subject()}
	if _, ok := r.byKey[key]; ok {
		return fmt.Errorf("%w: %s/%s",
			account.ErrAccountAlreadyExists, a.Provider(), a.Subject())
	}
	r.byKey[key] = a
	return nil
}

func (r *signInAccounts) created() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.createCnt
}

// insertOnNextCreate は次の Create の直前に、別の UserID を持つ同じ
// (provider, subject) のアカウントを割り込ませる。その Create は
// ErrAccountAlreadyExists で失敗する。
func (r *signInAccounts) insertOnNextCreate(
	t *testing.T, provider account.Provider, subject string, userID account.UserID,
) {
	t.Helper()

	r.mu.Lock()
	defer r.mu.Unlock()
	r.onCreate = func() {
		a, err := account.NewAccount(provider, subject, userID)
		if err != nil {
			t.Errorf("割り込ませるアカウントが不正: %v", err)
			return
		}
		r.mu.Lock()
		defer r.mu.Unlock()
		r.byKey[signInAccountKey{provider: provider, subject: subject}] = a
	}
}

// signInSessions はセッションの保存先。Create の回数を数える。
type signInSessions struct {
	mu        sync.Mutex
	byHash    map[account.TokenHash]*account.Session
	createCnt int
}

func newSignInSessions() *signInSessions {
	return &signInSessions{byHash: map[account.TokenHash]*account.Session{}}
}

func (r *signInSessions) Create(_ context.Context, s *account.Session) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.createCnt++
	r.byHash[s.TokenHash()] = s
	return nil
}

func (r *signInSessions) Delete(_ context.Context, hash account.TokenHash) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.byHash, hash)
	return nil
}

func (r *signInSessions) created() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.createCnt
}

// find は保存済みのセッションを引く。SessionReader ではないので
// このファイルの中だけで使う。
func (r *signInSessions) find(
	hash account.TokenHash, now time.Time,
) (*account.Session, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	s, ok := r.byHash[hash]
	if !ok || s.IsExpired(now) {
		return nil, account.ErrSessionNotFound
	}
	return s, nil
}

// signInPrograms は利用者ごとのプログラムの保存先。
//
// 既存の fakeProgram を使わないのは、あちらが UserID を無視して1つしか
// 持たないため。「その利用者の下に入ったか」「他人のものを上書きしないか」
// を検査したいので、鍵で分かれている必要がある。
type signInPrograms struct {
	mu     sync.Mutex
	byUser map[account.UserID]*program.Program
}

func newSignInPrograms() *signInPrograms {
	return &signInPrograms{byUser: map[account.UserID]*program.Program{}}
}

func (r *signInPrograms) Get(
	_ context.Context, userID account.UserID,
) (*program.Program, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	p, ok := r.byUser[userID]
	if !ok {
		return nil, fmt.Errorf("%w: %s", program.ErrProgramNotConfigured, userID)
	}
	return p, nil
}

func (r *signInPrograms) Save(
	_ context.Context, userID account.UserID, p *program.Program,
) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.byUser[userID] = p
	return nil
}

// failingProgramWriter は必ず失敗する保存口。
type failingProgramWriter struct{ err error }

func (w *failingProgramWriter) Save(
	_ context.Context, _ account.UserID, _ *program.Program,
) error {
	return fmt.Errorf("プログラムを保存できない: %w", w.err)
}

func mustUserID(t *testing.T) account.UserID {
	t.Helper()

	id, err := account.NewRandomUserID()
	if err != nil {
		t.Fatalf("UserID を採番できない: %v", err)
	}
	return id
}
