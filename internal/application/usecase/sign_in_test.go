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
	"github.com/dyoshyy/liftplan/internal/domain/training"
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

// githubIdentity は GitHub から来た identity。アドレスは指定した分だけ入る。
func githubIdentity(t *testing.T, email string) account.Identity {
	t.Helper()
	provider, subject := githubSubject()
	id, err := account.NewIdentity(provider, subject, account.NewEmail(email))
	if err != nil {
		t.Fatalf("identity が不正: %v", err)
	}
	return id
}

// googleIdentity は Google から来た identity。**別のプロバイダ・別の subject**。
func googleIdentity(t *testing.T, email string) account.Identity {
	t.Helper()
	id, err := account.NewIdentity(account.Google(), "google-sub-1", account.NewEmail(email))
	if err != nil {
		t.Fatalf("identity が不正: %v", err)
	}
	return id
}

var signInNow = time.Date(2026, 9, 21, 10, 0, 0, 0, time.UTC)

// 初回ログインで、アカウント・初期プログラム・セッションが揃うこと。
//
// どれか1つでも欠けると、ログインはできたのに使えない状態になる。
// 特にプログラムが無いと、画面は「設定してください」しか出せない。
func TestSignIn_FirstTimeCreatesEverything(t *testing.T) {
	f := newSignInFixture(t)
	provider, subject := githubSubject()

	token, err := f.signIn.Execute(context.Background(), githubIdentity(t, ""), signInNow)
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
	if a.UserID() == legacyUserID() {
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

	first, err := f.signIn.Execute(context.Background(), githubIdentity(t, ""), signInNow)
	if err != nil {
		t.Fatalf("初回ログインに失敗: %v", err)
	}
	second, err := f.signIn.Execute(
		context.Background(), githubIdentity(t, ""), signInNow.Add(time.Hour))
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
		context.Background(), githubIdentity(t, ""), signInNow); err != nil {
		t.Fatalf("初回ログインに失敗: %v", err)
	}
	a, err := f.accounts.Find(context.Background(), provider, subject)
	if err != nil {
		t.Fatalf("アカウントが作られていない: %v", err)
	}

	// 本人が頻度を変えた。シードの既定（3）とは違う値にする。
	changed := changeFrequency(t, f, a.UserID())

	if _, err := f.signIn.Execute(
		context.Background(), githubIdentity(t, ""), signInNow.Add(time.Hour)); err != nil {
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
	changed, err := program.NewProgram(freq, mustVolume(t, 6, 3), prog.SelectedExercises(), prog.DeclaredExercises(), "")
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

	token, err := f.signIn.Execute(context.Background(), githubIdentity(t, ""), signInNow)
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

	_, err := f.signIn.Execute(context.Background(), githubIdentity(t, ""), signInNow)
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

	// byEmail は確認済みアドレス→利用者。emailErr を入れると引きが失敗する。
	byEmail      map[string]account.UserID
	emailErr     error
	emailLookups int
}

func newSignInAccounts() *signInAccounts {
	return &signInAccounts{
		byKey:   map[signInAccountKey]*account.Account{},
		byEmail: map[string]account.UserID{},
	}
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

// FindUserByEmail は確認済みアドレスから利用者を引く。
//
// byEmail に無ければ見つからない。emailErr を入れておくと、引き方に
// 関わらずそのエラーを返す（曖昧・保存先の不調を作るため）。
func (r *signInAccounts) FindUserByEmail(
	_ context.Context, email account.Email,
) (account.UserID, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.emailLookups++
	if r.emailErr != nil {
		return account.UserID{}, r.emailErr
	}
	if email.IsZero() {
		return account.UserID{}, fmt.Errorf("%w: 空のアドレス", account.ErrAccountNotFound)
	}
	uid, ok := r.byEmail[email.String()]
	if !ok {
		return account.UserID{}, fmt.Errorf("%w: %s", account.ErrAccountNotFound, email)
	}
	return uid, nil
}

// FindByUser はログインでは使わない。口を満たすためだけに置く。
func (r *signInAccounts) FindByUser(context.Context, account.UserID) ([]*account.Account, error) {
	return nil, nil
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

// UpdateEmail はアドレスだけを書き直す。利用者はそのまま持ち越す。
func (r *signInAccounts) UpdateEmail(
	_ context.Context, provider account.Provider, subject string, email account.Email,
) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	key := signInAccountKey{provider: provider, subject: subject}
	a, ok := r.byKey[key]
	if !ok {
		return fmt.Errorf("%w: %s/%s", account.ErrAccountNotFound, provider, subject)
	}
	updated, err := account.NewAccount(a.Provider(), a.Subject(), a.UserID(), email)
	if err != nil {
		return err
	}
	r.byKey[key] = updated
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
		a, err := account.NewAccount(provider, subject, userID, account.Email{})
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

// legacyUserID はマイグレーション 0007 が既存の行を寄せた先。
//
// コードからは消した（もう誰も使わない）が、DBには残っている。
// **SignIn がこれを特別扱いしないこと**が、このファイルの検査の1つ。
// 引き継ぎは初回ログインの前に手で流す INSERT の仕事（docs/deploy.md）。
func legacyUserID() account.UserID {
	return mustTestUserID("8d5e743e-f1b0-4430-9998-89d313e89da8")
}

// ---- プロバイダをまたいで結ぶ ----

// 確認済みアドレスが一致するなら、同じ利用者になること。
//
// **これが無いと、GitHub で入った人が Google を押した瞬間に記録が
// 空になる。**本人から見れば「記録が消えた」で、原因は分からない。
func TestSignIn_LinksProvidersByVerifiedEmail(t *testing.T) {
	f := newSignInFixture(t)
	ctx := context.Background()

	// GitHub で初回ログイン。
	if _, err := f.signIn.Execute(ctx, githubIdentity(t, "me@example.com"), signInNow); err != nil {
		t.Fatalf("GitHub のログインに失敗: %v", err)
	}
	provider, subject := githubSubject()
	first, err := f.accounts.Find(ctx, provider, subject)
	if err != nil {
		t.Fatalf("GitHub のアカウントが無い: %v", err)
	}

	// 同じアドレスの利用者として引けるようにしておく（保存先が持つ状態）。
	f.accounts.byEmail["me@example.com"] = first.UserID()

	// Google で初回ログイン。別のプロバイダ・別の subject。
	if _, err := f.signIn.Execute(ctx, googleIdentity(t, "me@example.com"), signInNow); err != nil {
		t.Fatalf("Google のログインに失敗: %v", err)
	}
	second, err := f.accounts.Find(ctx, account.Google(), "google-sub-1")
	if err != nil {
		t.Fatalf("Google のアカウントが無い: %v", err)
	}

	if second.UserID() != first.UserID() {
		t.Errorf("利用者が %q。GitHub と同じ %q のはず",
			second.UserID(), first.UserID())
	}
	// アカウントは2つ。結ぶのは利用者であって、アカウントではない。
	if f.accounts.created() != 2 {
		t.Errorf("アカウントが %d 件。2件のはず", f.accounts.created())
	}
}

// 結んだ先のプログラムを上書きしないこと。
//
// 上書きすると、GitHub で設定を整えた人が Google で入り直した瞬間に
// 設定が初期値へ戻る。これは記録が消えるのと同じくらい困る。
func TestSignIn_DoesNotOverwriteTheProgramWhenLinking(t *testing.T) {
	f := newSignInFixture(t)
	ctx := context.Background()

	if _, err := f.signIn.Execute(ctx, githubIdentity(t, "me@example.com"), signInNow); err != nil {
		t.Fatalf("GitHub のログインに失敗: %v", err)
	}
	provider, subject := githubSubject()
	first, _ := f.accounts.Find(ctx, provider, subject)
	f.accounts.byEmail["me@example.com"] = first.UserID()

	want := changeFrequency(t, f, first.UserID())

	if _, err := f.signIn.Execute(ctx, googleIdentity(t, "me@example.com"), signInNow); err != nil {
		t.Fatalf("Google のログインに失敗: %v", err)
	}

	got, err := f.programs.Get(ctx, first.UserID())
	if err != nil {
		t.Fatalf("プログラムが読めない: %v", err)
	}
	if got.Frequency().PerWeek() != want {
		t.Errorf("頻度が %d。%d のはず（結んだときに初期値へ戻った）",
			got.Frequency().PerWeek(), want)
	}
}

// 結べないときは、結ばずに新しい利用者を作ること。失敗にはしない。
func TestSignIn_DoesNotLinkWhenItCannotBeSure(t *testing.T) {
	cases := []struct {
		name  string
		email string
		// setup は保存先の状態を作る。
		setup func(*signInFixture)
	}{
		{
			// アドレスが取れていない。結ぶ材料が無い。
			name: "確認済みアドレスが無い", email: "",
			setup: func(*signInFixture) {},
		},
		{
			// 同じアドレスに複数の利用者。どちらか分からないまま選ぶと、
			// 選び方次第で他人の記録に結びつく。
			name: "同じアドレスに複数の利用者がいる", email: "me@example.com",
			setup: func(f *signInFixture) { f.accounts.emailErr = account.ErrAmbiguousEmail },
		},
		{
			// そのアドレスの利用者はまだ居ない。
			name: "一致する利用者がいない", email: "me@example.com",
			setup: func(*signInFixture) {},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := newSignInFixture(t)
			ctx := context.Background()
			c.setup(f)

			token, err := f.signIn.Execute(ctx, googleIdentity(t, c.email), signInNow)
			if err != nil {
				t.Fatalf("ログインに失敗: %v", err)
			}
			if token.String() == "" {
				t.Fatal("トークンが空")
			}

			a, err := f.accounts.Find(ctx, account.Google(), "google-sub-1")
			if err != nil {
				t.Fatalf("アカウントが無い: %v", err)
			}
			if a.UserID() == (account.UserID{}) {
				t.Error("利用者が採番されていない")
			}
		})
	}
}

// アドレスを引けなかった（保存先の不調）ときは、新しい利用者を作らない。
//
// **ここを「見つからなかった」と同じ扱いにしてはいけない。**DB が一瞬
// 応えなかっただけで、本人の記録から切り離された利用者が黙って生まれる。
// あとから直すには、どの行が誤って作られたかを人が突き合わせるしかない。
func TestSignIn_FailsWhenTheEmailLookupBreaks(t *testing.T) {
	f := newSignInFixture(t)
	f.accounts.emailErr = fmt.Errorf("%w: 接続できない", training.ErrRepositoryUnavailable)

	_, err := f.signIn.Execute(context.Background(), googleIdentity(t, "me@example.com"), signInNow)
	if err == nil {
		t.Fatal("引きが壊れているのにログインが成立した")
	}
	if f.accounts.created() != 0 {
		t.Errorf("アカウントを %d 件作っている。0件のはず", f.accounts.created())
	}
}

// 既にあるアカウントの利用者は、決して書き換えないこと。
//
// 書き換える経路があると、**メールアドレスを他人のものに変えて
// 入り直すだけで、他人の記録に入れる。**結ぶのは作るときだけ。
func TestSignIn_NeverRelinksAnExistingAccount(t *testing.T) {
	f := newSignInFixture(t)
	ctx := context.Background()

	if _, err := f.signIn.Execute(ctx, githubIdentity(t, "me@example.com"), signInNow); err != nil {
		t.Fatalf("初回のログインに失敗: %v", err)
	}
	provider, subject := githubSubject()
	first, _ := f.accounts.Find(ctx, provider, subject)

	// 別人のアドレスが、そのアドレスの持ち主として引けるようにする。
	other, err := account.NewRandomUserID()
	if err != nil {
		t.Fatalf("採番できない: %v", err)
	}
	f.accounts.byEmail["someone-else@example.com"] = other

	// 同じ (provider, subject) で、別のアドレスを持って入り直す。
	if _, err := f.signIn.Execute(ctx, githubIdentity(t, "someone-else@example.com"), signInNow); err != nil {
		t.Fatalf("2回目のログインに失敗: %v", err)
	}

	again, _ := f.accounts.Find(ctx, provider, subject)
	if again.UserID() != first.UserID() {
		t.Errorf("利用者が %q に変わった。%q のままのはず", again.UserID(), first.UserID())
	}
	if f.accounts.emailLookups != 1 {
		t.Errorf("アドレスを %d 回引いている。作るときの1回だけのはず", f.accounts.emailLookups)
	}
}

// 2回目以降のログインで、プロバイダが返したアドレスを書き直すこと。
//
// アドレスの列（0010）より前に作られたアカウントは空のままで、書き直さないと
// 設定画面にいつまでもアドレスが出ない。本番の2行はどちらもこの形だった。
func TestSignIn_FillsTheEmailOfAnExistingAccount(t *testing.T) {
	f := newSignInFixture(t)
	ctx := context.Background()
	provider, subject := githubSubject()

	if _, err := f.signIn.Execute(ctx, githubIdentity(t, ""), signInNow); err != nil {
		t.Fatalf("初回ログインに失敗: %v", err)
	}
	first, _ := f.accounts.Find(ctx, provider, subject)

	if _, err := f.signIn.Execute(ctx, githubIdentity(t, "me@example.com"), signInNow); err != nil {
		t.Fatalf("2回目のログインに失敗: %v", err)
	}

	again, _ := f.accounts.Find(ctx, provider, subject)
	if again.Email() != account.NewEmail("me@example.com") {
		t.Errorf("アドレスが %q。me@example.com のはず", again.Email())
	}
	if again.UserID() != first.UserID() {
		t.Errorf("利用者が %q に変わった。%q のままのはず", again.UserID(), first.UserID())
	}
}

// プロバイダがアドレスを返さなかったときは、持っているアドレスを消さない。
// 返さないのは「取れなかった」で、「無くなった」ではない。
func TestSignIn_KeepsTheEmailWhenTheProviderSendsNone(t *testing.T) {
	f := newSignInFixture(t)
	ctx := context.Background()
	provider, subject := githubSubject()

	if _, err := f.signIn.Execute(ctx, githubIdentity(t, "me@example.com"), signInNow); err != nil {
		t.Fatalf("初回ログインに失敗: %v", err)
	}
	if _, err := f.signIn.Execute(ctx, githubIdentity(t, ""), signInNow); err != nil {
		t.Fatalf("2回目のログインに失敗: %v", err)
	}

	again, _ := f.accounts.Find(ctx, provider, subject)
	if again.Email() != account.NewEmail("me@example.com") {
		t.Errorf("アドレスが %q。me@example.com のまま残るはず", again.Email())
	}
}
