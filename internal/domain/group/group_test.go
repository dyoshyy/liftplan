package group_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/dyoshyy/liftplan/internal/domain/account"
	"github.com/dyoshyy/liftplan/internal/domain/group"
)

var (
	at    = time.Date(2026, 10, 11, 12, 0, 0, 0, time.UTC)
	userA = mustUserID("11111111-1111-4111-8111-111111111111")
	userB = mustUserID("22222222-2222-4222-8222-222222222222")
	userC = mustUserID("33333333-3333-4333-8333-333333333333")
)

func mustUserID(s string) account.UserID {
	id, err := account.NewUserID(s)
	if err != nil {
		panic(err)
	}
	return id
}

func mustDisplayName(t *testing.T, s string) group.DisplayName {
	t.Helper()
	n, err := group.NewDisplayName(s)
	if err != nil {
		t.Fatalf("NewDisplayName(%q): %v", s, err)
	}
	return n
}

func mustMember(t *testing.T, user account.UserID, name string, joinedAt time.Time) group.Member {
	t.Helper()
	m, err := group.NewMember(user, mustDisplayName(t, name), joinedAt)
	if err != nil {
		t.Fatalf("NewMember: %v", err)
	}
	return m
}

// newGroup は members を持つグループを作る。
func newGroup(t *testing.T, members ...group.Member) *group.Group {
	t.Helper()
	id, err := group.NewRandomID()
	if err != nil {
		t.Fatal(err)
	}
	name, err := group.NewName("ジム仲間")
	if err != nil {
		t.Fatal(err)
	}
	token, err := group.NewInviteToken()
	if err != nil {
		t.Fatal(err)
	}
	g, err := group.New(group.Params{ID: id, Name: name, Invite: token, Members: members, CreatedAt: at})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return g
}

func userIDs(members []group.Member) []account.UserID {
	out := make([]account.UserID, 0, len(members))
	for _, m := range members {
		out = append(out, m.UserID())
	}
	return out
}

func TestGroup_VisibleTo(t *testing.T) {
	cases := []struct {
		name    string
		members []account.UserID
		viewer  account.UserID
		want    []account.UserID
		wantErr error
	}{
		{
			// 自分も含めて全員。自分の行が無いと「自分だけ空いている」が見えない。
			name:    "メンバーには自分を含む全員が見える",
			members: []account.UserID{userA, userB, userC},
			viewer:  userB,
			want:    []account.UserID{userA, userB, userC},
		},
		{
			// ここが破れると、グループの ID を知っているだけで他人の記録が読める。
			name:    "メンバーでない人には何も見せない",
			members: []account.UserID{userA, userB},
			viewer:  userC,
			wantErr: group.ErrNotMember,
		},
		{
			// 最後の1人が同時に抜けて残った空のグループ。誰でも見られるに
			// 倒すと、空のグループの ID を知る人が「見られる側」に回る。
			name:    "空のグループは誰にも見せない",
			members: nil,
			viewer:  userA,
			wantErr: group.ErrNotMember,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			ms := make([]group.Member, 0, len(c.members))
			for i, u := range c.members {
				ms = append(ms, mustMember(t, u, "メンバー", at.Add(time.Duration(i)*time.Minute)))
			}
			g := newGroup(t, ms...)

			got, err := g.VisibleTo(c.viewer)
			if c.wantErr != nil {
				if !errors.Is(err, c.wantErr) {
					t.Fatalf("エラーが %v。%v のはず", err, c.wantErr)
				}
				if len(got.UserIDs()) != 0 {
					t.Errorf("エラーなのに %d 人を返した", len(got.UserIDs()))
				}
				return
			}
			if err != nil {
				t.Fatalf("エラー: %v", err)
			}
			if gotIDs := got.UserIDs(); !equalIDs(gotIDs, c.want) {
				t.Errorf("見えるのが %v。%v のはず", gotIDs, c.want)
			}
		})
	}
}

func equalIDs(a, b []account.UserID) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestGroup_Join(t *testing.T) {
	t.Run("新しい人は参加順の最後に加わる", func(t *testing.T) {
		g := newGroup(t, mustMember(t, userA, "よし", at))

		next, m, joined, err := g.Join(userB, mustDisplayName(t, "たろう"), at.Add(time.Hour))
		if err != nil {
			t.Fatalf("エラー: %v", err)
		}
		if !joined {
			t.Error("新しく加わったのに joined が false")
		}
		if m.UserID() != userB || m.DisplayName().String() != "たろう" {
			t.Errorf("加えたメンバーが %v/%v", m.UserID(), m.DisplayName())
		}
		if got := userIDs(next.Members()); !equalIDs(got, []account.UserID{userA, userB}) {
			t.Errorf("メンバーが %v。[A B] のはず", got)
		}
		if !next.IsMember(userB) {
			t.Error("加わった人が IsMember で false")
		}
		// 元の値は変わらない。変わると、同じ *Group を持つ呼び出し側が黙って巻き込まれる。
		if g.IsMember(userB) {
			t.Error("元のグループまで書き換わった")
		}
	})

	// 二度押し・通信のやり直しで2行にならないこと。表示名も最初のまま。
	t.Run("既にメンバーなら何も変えない", func(t *testing.T) {
		g := newGroup(t, mustMember(t, userA, "よし", at))

		next, m, joined, err := g.Join(userA, mustDisplayName(t, "別の名前"), at.Add(time.Hour))
		if err != nil {
			t.Fatalf("エラー: %v", err)
		}
		if joined {
			t.Error("既にメンバーなのに joined が true")
		}
		if m.DisplayName().String() != "よし" {
			t.Errorf("返したメンバーの表示名が %q。元の「よし」のはず", m.DisplayName())
		}
		ms := next.Members()
		if len(ms) != 1 || ms[0].DisplayName().String() != "よし" {
			t.Errorf("メンバーが変わった: %v", ms)
		}
	})
}

func TestGroup_Leave(t *testing.T) {
	t.Run("抜けた人は見られる側からも見る側からも消える", func(t *testing.T) {
		g := newGroup(t, mustMember(t, userA, "よし", at), mustMember(t, userB, "たろう", at.Add(time.Minute)))

		next, err := g.Leave(userB)
		if err != nil {
			t.Fatalf("エラー: %v", err)
		}
		if next.IsMember(userB) {
			t.Error("抜けた人がまだメンバー")
		}
		if _, err := next.VisibleTo(userB); !errors.Is(err, group.ErrNotMember) {
			t.Errorf("抜けた人に見せている: %v", err)
		}
		if !g.IsMember(userB) {
			t.Error("元のグループまで書き換わった")
		}
		a, err := next.VisibleTo(userA)
		if err != nil {
			t.Fatal(err)
		}
		if got := a.UserIDs(); !equalIDs(got, []account.UserID{userA}) {
			t.Errorf("残った人に見えるのが %v。[A] のはず", got)
		}
		if next.IsEmpty() {
			t.Error("1人残っているのに空")
		}
	})

	t.Run("最後の1人が抜けたら空になる", func(t *testing.T) {
		g := newGroup(t, mustMember(t, userA, "よし", at))
		next, err := g.Leave(userA)
		if err != nil {
			t.Fatal(err)
		}
		if !next.IsEmpty() {
			t.Error("誰もいないのに空ではない")
		}
	})

	t.Run("メンバーでない人は抜けられない", func(t *testing.T) {
		g := newGroup(t, mustMember(t, userA, "よし", at))
		if _, err := g.Leave(userC); !errors.Is(err, group.ErrNotMember) {
			t.Errorf("エラーが %v。ErrNotMember のはず", err)
		}
	})
}

func TestGroup_RotateInvite(t *testing.T) {
	next, err := group.NewInviteToken()
	if err != nil {
		t.Fatal(err)
	}

	t.Run("メンバーなら作り直せる", func(t *testing.T) {
		g := newGroup(t, mustMember(t, userA, "よし", at))
		before := g.Invite()
		got, err := g.RotateInvite(userA, next)
		if err != nil {
			t.Fatalf("エラー: %v", err)
		}
		if got.Invite() != next {
			t.Errorf("トークンが %q。%q に替わるはず", got.Invite(), next)
		}
		if g.Invite() != before {
			t.Error("元のグループまで書き換わった")
		}
	})

	// 外の人が作り直せると、メンバーの持っているリンクを無効にできる。
	t.Run("メンバーでない人は作り直せない", func(t *testing.T) {
		g := newGroup(t, mustMember(t, userA, "よし", at))
		if _, err := g.RotateInvite(userC, next); !errors.Is(err, group.ErrNotMember) {
			t.Errorf("エラーが %v。ErrNotMember のはず", err)
		}
	})
}

func TestNew(t *testing.T) {
	id, _ := group.NewRandomID()
	name, _ := group.NewName("ジム仲間")
	token, _ := group.NewInviteToken()
	a := mustMember(t, userA, "よし", at)

	cases := []struct {
		name string
		p    group.Params
	}{
		{name: "ID が無い", p: group.Params{Name: name, Invite: token, Members: []group.Member{a}, CreatedAt: at}},
		{name: "名前が無い", p: group.Params{ID: id, Invite: token, Members: []group.Member{a}, CreatedAt: at}},
		{name: "招待トークンが無い", p: group.Params{ID: id, Name: name, Members: []group.Member{a}, CreatedAt: at}},
		{name: "作った時刻が無い", p: group.Params{ID: id, Name: name, Invite: token, Members: []group.Member{a}}},
		{
			// 同じ人が2行あると、抜けても1行残って見え続ける。
			name: "同じ人が二度いる",
			p: group.Params{ID: id, Name: name, Invite: token, CreatedAt: at,
				Members: []group.Member{a, mustMember(t, userA, "よし2", at.Add(time.Minute))}},
		},
	}
	for _, c := range cases {
		t.Run(c.name+"なら作れない", func(t *testing.T) {
			if _, err := group.New(c.p); !errors.Is(err, group.ErrInvalidGroup) {
				t.Errorf("エラーが %v。ErrInvalidGroup のはず", err)
			}
		})
	}

	// 保存先の並びに依らず、参加順で返す。画面の行の並びがここで決まる。
	t.Run("メンバーは参加順に並べ直す", func(t *testing.T) {
		b := mustMember(t, userB, "たろう", at.Add(-time.Hour))
		g, err := group.New(group.Params{ID: id, Name: name, Invite: token, CreatedAt: at,
			Members: []group.Member{a, b}})
		if err != nil {
			t.Fatal(err)
		}
		if got := userIDs(g.Members()); !equalIDs(got, []account.UserID{userB, userA}) {
			t.Errorf("並びが %v。[B A] のはず", got)
		}
	})
}

func TestNewDisplayName(t *testing.T) {
	cases := []struct {
		name    string
		in      string
		want    string
		wantErr bool
	}{
		{name: "前後の空白は除く", in: "  よし ", want: "よし"},
		{name: "空は弾く", in: "", wantErr: true},
		{name: "空白だけは弾く", in: "　 ", wantErr: true},
		// 文字数で数える。バイト数だと日本語7文字で21バイトを超えて弾かれる。
		{name: "20文字までは通す", in: strings.Repeat("あ", 20), want: strings.Repeat("あ", 20)},
		{name: "21文字は弾く", in: strings.Repeat("あ", 21), wantErr: true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := group.NewDisplayName(c.in)
			if c.wantErr {
				if !errors.Is(err, group.ErrInvalidName) {
					t.Errorf("エラーが %v。ErrInvalidName のはず", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got.String() != c.want {
				t.Errorf("%q。%q のはず", got, c.want)
			}
		})
	}
}

func TestParseID(t *testing.T) {
	cases := []struct {
		name    string
		in      string
		want    string
		wantErr bool
	}{
		{name: "大文字は小文字に揃える", in: "AAAAAAAA-1111-4111-8111-111111111111", want: "aaaaaaaa-1111-4111-8111-111111111111"},
		{name: "ハイフン抜きは弾く", in: "aaaaaaaa111141118111111111111111", wantErr: true},
		{name: "16進でない文字は弾く", in: "gggggggg-1111-4111-8111-111111111111", wantErr: true},
		{name: "空は弾く", in: "", wantErr: true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := group.ParseID(c.in)
			if c.wantErr {
				if !errors.Is(err, group.ErrInvalidID) {
					t.Errorf("エラーが %v。ErrInvalidID のはず", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got.String() != c.want {
				t.Errorf("%q。%q のはず", got, c.want)
			}
		})
	}

	t.Run("採番した ID は読み戻せる", func(t *testing.T) {
		id, err := group.NewRandomID()
		if err != nil {
			t.Fatal(err)
		}
		if back, err := group.ParseID(id.String()); err != nil || back != id {
			t.Errorf("読み戻せない: %v %v", back, err)
		}
	})
}
