package account_test

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/dyoshyy/liftplan/internal/domain/account"
)

// テストの基準時刻。time.Now() を使うと、境界のケースが「たまたま通る／
// たまたま落ちる」になる。
var at = time.Date(2026, 9, 21, 12, 0, 0, 0, time.UTC)

// 期限の境界。ここを外すと、切れたセッションで他人の記録に入れる。
//
// 有効なのは now < expiresAt の間だけ（期限ちょうどは切れている扱い）。
func TestSession_IsExpired(t *testing.T) {
	expiresAt := at.Add(account.SessionLifetime)

	cases := []struct {
		name string
		now  time.Time
		want bool
	}{
		{name: "発行直後は切れていない", now: at, want: false},
		{
			name: "期限の1ナノ秒前は切れていない",
			now:  expiresAt.Add(-time.Nanosecond), want: false,
		},
		{
			// 境界。「以下」と「未満」を取り違えると、ここだけが変わる。
			name: "期限ちょうどは切れている",
			now:  expiresAt, want: true,
		},
		{
			name: "期限の1ナノ秒後は切れている",
			now:  expiresAt.Add(time.Nanosecond), want: true,
		},
		{
			// 90日。ジムで毎回ログインし直させないための長さで、
			// これを1日に縮めると毎回ログイン画面に戻る。
			name: "発行から89日は切れていない",
			now:  at.Add(89 * 24 * time.Hour), want: false,
		},
		{
			name: "発行から91日は切れている",
			now:  at.Add(91 * 24 * time.Hour), want: true,
		},
		{
			// タイムゾーンが違っても同じ瞬間なら同じ答えになること。
			// UTC へ寄せていないと、9時間ずれて期限切れが通る。
			name: "別のタイムゾーンで表した期限ちょうども切れている",
			now:  expiresAt.In(time.FixedZone("JST", 9*60*60)), want: true,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, s, err := account.IssueSession(testUser, at)
			if err != nil {
				t.Fatalf("発行に失敗: %v", err)
			}

			if got := s.IsExpired(c.now); got != c.want {
				t.Errorf("IsExpired(%v) が %v。%v のはず", c.now, got, c.want)
			}
		})
	}
}

// 発行したセッションの期限が「現在時刻＋90日」であること。
func TestIssueSession_ExpiresIn90Days(t *testing.T) {
	_, s, err := account.IssueSession(testUser, at)
	if err != nil {
		t.Fatalf("発行に失敗: %v", err)
	}

	want := at.Add(90 * 24 * time.Hour)
	if !s.ExpiresAt().Equal(want) {
		t.Errorf("期限が %v。%v のはず", s.ExpiresAt(), want)
	}
	if s.UserID() != testUser {
		t.Errorf("持ち主が %q。%q のはず", s.UserID(), testUser)
	}
}

// 発行したトークンは、ハッシュにしか痕跡を残さない。
//
// Session がトークンそのものを持つと、DB へ書く実装がいつか素通しで
// 保存しうる。持たない形にしておけば、保存しようにも取り出せない。
func TestIssueSession_KeepsOnlyTheHash(t *testing.T) {
	token, s, err := account.IssueSession(testUser, at)
	if err != nil {
		t.Fatalf("発行に失敗: %v", err)
	}

	if token.String() == "" {
		t.Fatal("トークンが空である")
	}
	// Session を丸ごと文字列にしてもトークンが出てこないこと。
	// ここに出るなら、ログに出した瞬間に漏れる。
	if dump := fmt.Sprintf("%+v", *s); strings.Contains(dump, token.String()) {
		t.Errorf("Session の中にトークンそのものが入っている: %s", dump)
	}
	if s.TokenHash() != token.Hash() {
		t.Errorf("保持しているハッシュがトークンのハッシュと一致しない")
	}
}

// 2回発行したトークンが違うこと。
//
// 乱数源を固定値に取り違えると、全員が同じトークンを持ち、誰でも
// 他人のセッションで入れる。
func TestNewSessionToken_DiffersEveryTime(t *testing.T) {
	const n = 100

	seen := make(map[string]bool, n)
	for range n {
		token, err := account.NewSessionToken()
		if err != nil {
			t.Fatalf("生成に失敗: %v", err)
		}
		// 32バイトを base64（パディング無し）で書くと43文字。
		// 短くすると総当たりで当たるようになる。
		if len(token.String()) != 43 {
			t.Fatalf("トークンの長さが %d。43 のはず（32バイト）", len(token.String()))
		}
		if seen[token.String()] {
			t.Fatalf("同じトークンが2度出た: %s", token.String())
		}
		seen[token.String()] = true
	}
}

// ハッシュが SHA-256 であること。
//
// 実装を md5 や「トークンそのもの」に取り替えても、往復するだけの
// テストは緑のまま通る。値そのものを固定する。
func TestSessionToken_Hash(t *testing.T) {
	token, err := account.ParseSessionToken("liftplan")
	if err != nil {
		t.Fatalf("エラーが返った: %v", err)
	}

	sum := sha256.Sum256([]byte("liftplan"))
	want := hex.EncodeToString(sum[:])

	if got := token.Hash().String(); got != want {
		t.Errorf("ハッシュが %q。SHA-256 の %q のはず", got, want)
	}
	if strings.Contains(token.Hash().String(), "liftplan") {
		t.Error("ハッシュにトークンがそのまま入っている")
	}
}

func TestParseSessionToken(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string // 空なら不正
	}{
		{name: "端末から来た文字列をそのまま受け取る", in: "abc.def", want: "abc.def"},
		{name: "前後の空白は落とす", in: " abc\n", want: "abc"},
		{
			// 空を通すと「空文字のハッシュ」で引く経路ができる。
			// Authorization ヘッダが空のまま通った場合がこれ。
			name: "空は受け取らない", in: "",
		},
		{name: "空白だけは受け取らない", in: "   "},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := account.ParseSessionToken(c.in)

			if c.want == "" {
				if !errors.Is(err, account.ErrInvalidSessionToken) {
					t.Fatalf("エラーが %v。ErrInvalidSessionToken のはず", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("エラーが返った: %v", err)
			}
			if got.String() != c.want {
				t.Errorf("トークンが %q。%q のはず", got.String(), c.want)
			}
		})
	}
}

func TestNewTokenHash(t *testing.T) {
	valid := strings.Repeat("ab", 32) // 64桁の16進

	cases := []struct {
		name string
		in   string
		want string // 空なら不正
	}{
		{name: "64桁の16進を受け取る", in: valid, want: valid},
		{
			// Postgres から読み戻す値は小文字で入れている想定だが、
			// 大文字のまま通すと同じセッションが2つのハッシュを持つ。
			name: "大文字は小文字に揃える",
			in:   strings.ToUpper(valid), want: valid,
		},
		{name: "前後の空白は落とす", in: " " + valid + "\n", want: valid},
		{name: "空は受け取らない", in: ""},
		{name: "短いものは受け取らない", in: valid[:63]},
		{name: "長いものは受け取らない", in: valid + "a"},
		{
			// 長さだけ見ていると通る。
			name: "16進でない文字は受け取らない",
			in:   valid[:62] + "zz",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := account.NewTokenHash(c.in)

			if c.want == "" {
				if !errors.Is(err, account.ErrInvalidSessionToken) {
					t.Fatalf("エラーが %v。ErrInvalidSessionToken のはず", err)
				}
				if got != (account.TokenHash{}) {
					t.Errorf("不正な入力から %q が返った。ゼロ値のはず", got.String())
				}
				return
			}
			if err != nil {
				t.Fatalf("エラーが返った: %v", err)
			}
			if got.String() != c.want {
				t.Errorf("ハッシュが %q。%q のはず", got.String(), c.want)
			}
		})
	}
}

// DB から読み戻したハッシュが、トークンから作ったものと == で等しいこと。
//
// ここが等しくないと、リポジトリが自分で保存した行を引けない。
func TestNewSession_RoundTripsTheHash(t *testing.T) {
	token, issued, err := account.IssueSession(testUser, at)
	if err != nil {
		t.Fatalf("発行に失敗: %v", err)
	}

	hash, err := account.NewTokenHash(issued.TokenHash().String())
	if err != nil {
		t.Fatalf("ハッシュを読み戻せない: %v", err)
	}
	if hash != token.Hash() {
		t.Errorf("読み戻したハッシュが元と等しくない")
	}

	restored, err := account.NewSession(hash, issued.UserID(), issued.ExpiresAt())
	if err != nil {
		t.Fatalf("組み立て直せない: %v", err)
	}
	if !restored.ExpiresAt().Equal(issued.ExpiresAt()) {
		t.Errorf("期限が %v。%v のはず", restored.ExpiresAt(), issued.ExpiresAt())
	}
	if restored.UserID() != issued.UserID() {
		t.Errorf("持ち主が %q。%q のはず", restored.UserID(), issued.UserID())
	}
}

func TestNewSession_RejectsIncompleteValues(t *testing.T) {
	hash, err := account.NewTokenHash(strings.Repeat("ab", 32))
	if err != nil {
		t.Fatalf("ハッシュを作れない: %v", err)
	}

	cases := []struct {
		name      string
		hash      account.TokenHash
		userID    account.UserID
		expiresAt time.Time
	}{
		{
			// ゼロ値のハッシュを許すと、空文字で引ける行ができる。
			name:   "ハッシュがゼロ値なら作れない",
			userID: testUser, expiresAt: at,
		},
		{
			name: "利用者の識別子がゼロ値なら作れない",
			hash: hash, expiresAt: at,
		},
		{
			// 期限ゼロ値は「切れている」側に倒れるので害は小さいが、
			// 期限を渡し忘れた発行が通る形を残さない。
			name: "期限がゼロ値なら作れない",
			hash: hash, userID: testUser,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := account.NewSession(c.hash, c.userID, c.expiresAt)

			if !errors.Is(err, account.ErrInvalidSession) {
				t.Fatalf("エラーが %v。ErrInvalidSession のはず", err)
			}
			if got != nil {
				t.Error("不正な入力からセッションが返った")
			}
		})
	}
}
