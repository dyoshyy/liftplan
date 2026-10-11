package query_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/dyoshyy/liftplan/internal/application/apperror"
	"github.com/dyoshyy/liftplan/internal/application/query"
	"github.com/dyoshyy/liftplan/internal/domain/account"
	"github.com/dyoshyy/liftplan/internal/domain/group"
	"github.com/dyoshyy/liftplan/internal/domain/training"
)

var (
	groupAt  = time.Date(2026, 10, 11, 12, 0, 0, 0, time.UTC)
	friend   = mustUser("22222222-2222-4222-8222-222222222222")
	outsider = mustUser("33333333-3333-4333-8333-333333333333")
)

func mustUser(s string) account.UserID {
	id, err := account.NewUserID(s)
	if err != nil {
		panic(err)
	}
	return id
}

type groupFixture struct {
	activity *fakeActivity
	q        *query.Groups
	g        *group.Group
}

// newGroupFixture は testUser（よし）と friend（たろう）のグループを作る。
// outsider はどこにも入っていない。
func newGroupFixture(t *testing.T) groupFixture {
	t.Helper()
	activity := &fakeActivity{}

	member := func(u account.UserID, name string, at time.Time) group.Member {
		n, _ := group.NewDisplayName(name)
		m, err := group.NewMember(u, n, at)
		if err != nil {
			t.Fatal(err)
		}
		return m
	}
	id, _ := group.NewRandomID()
	name, _ := group.NewName("ジム仲間")
	token, _ := group.NewInviteToken()
	g, err := group.New(group.Params{ID: id, Name: name, Invite: token, CreatedAt: groupAt,
		Members: []group.Member{member(testUser, "よし", groupAt), member(friend, "たろう", groupAt.Add(time.Minute))}})
	if err != nil {
		t.Fatal(err)
	}
	return groupFixture{activity: activity, q: query.NewGroups(fakeGroups{all: []*group.Group{g}}, activity), g: g}
}

// record はその人がその日にやった種目名を足す。同じ名前を二度渡せば
// 読み口が重複を消さなかった場合と同じ形になる（重複を消すのは読み口の契約）。
func (f groupFixture) record(user account.UserID, date training.Date, names ...string) {
	for _, n := range names {
		f.activity.All = append(f.activity.All, group.NewActivity(user, date, n))
	}
}

// weekString は週を「名前(自分なら*) 日:種目,種目」の行にする。
func weekString(w query.GroupWeek) string {
	var b strings.Builder
	for _, m := range w.Members {
		b.WriteString(m.DisplayName)
		if m.IsMe {
			b.WriteString("*")
		}
		for _, d := range m.Days {
			fmt.Fprintf(&b, " %s:%s", d.Date, strings.Join(d.Exercises, ","))
		}
		b.WriteString("\n")
	}
	return b.String()
}

func TestGroups_Week(t *testing.T) {
	ctx := context.Background()
	// 2026-10-14 は水曜。週は 12（月）〜18（日）。
	wednesday := training.MustDate(2026, time.October, 14)

	t.Run("メンバーごとに、その週にやった日と種目名を参加順で返す", func(t *testing.T) {
		f := newGroupFixture(t)
		f.record(testUser, training.MustDate(2026, time.October, 12), "スクワット", "ベンチプレス")
		f.record(testUser, training.MustDate(2026, time.October, 11), "デッドリフト") // 前の週の日曜
		f.record(friend, training.MustDate(2026, time.October, 18), "チンニング")
		f.record(friend, training.MustDate(2026, time.October, 19), "ベンチプレス") // 次の週の月曜
		f.record(outsider, training.MustDate(2026, time.October, 13), "ベンチプレス")

		w, err := f.q.Week(ctx, friend, f.g.ID().String(), wednesday)
		if err != nil {
			t.Fatalf("読めない: %v", err)
		}
		if w.From.String() != "2026-10-12" || w.To.String() != "2026-10-18" {
			t.Errorf("週が %s〜%s。2026-10-12〜18 のはず", w.From, w.To)
		}
		// 種目名は名前の昇順。outsider は出ない。
		want := "よし 2026-10-12:スクワット,ベンチプレス\nたろう* 2026-10-18:チンニング\n"
		if got := weekString(w); got != want {
			t.Errorf("週が\n%s\nのはずが\n%s", want, got)
		}
	})

	// 日曜を週の頭にすると、日曜の記録が次の週に飛ぶ。
	t.Run("日曜に開いても、その日曜で終わる週を返す", func(t *testing.T) {
		f := newGroupFixture(t)
		w, err := f.q.Week(ctx, testUser, f.g.ID().String(), training.MustDate(2026, time.October, 18))
		if err != nil {
			t.Fatal(err)
		}
		if w.From.String() != "2026-10-12" || w.To.String() != "2026-10-18" {
			t.Errorf("週が %s〜%s。2026-10-12〜18 のはず", w.From, w.To)
		}
	})

	// 本文まで同じにする。違うと、隠したいグループの存在が本文から分かる。
	t.Run("メンバーでない人には、グループが無いときと同じエラーを返す", func(t *testing.T) {
		f := newGroupFixture(t)
		other, _ := group.NewRandomID()

		_, notMember := f.q.Week(ctx, outsider, f.g.ID().String(), wednesday)
		_, missing := f.q.Week(ctx, outsider, other.String(), wednesday)
		_, malformed := f.q.Week(ctx, outsider, "not-a-uuid", wednesday)

		for name, err := range map[string]error{"メンバーでない": notMember, "無い": missing, "形が不正": malformed} {
			if !errors.Is(err, apperror.ErrGroupNotFound) {
				t.Errorf("%s: %v。ErrGroupNotFound のはず", name, err)
				continue
			}
			if err.Error() != missing.Error() {
				t.Errorf("%s: 本文が %q。無いときの %q と同じのはず", name, err, missing)
			}
		}
		if strings.Contains(notMember.Error(), outsider.String()) {
			t.Errorf("本文に利用者の ID が出ている: %q", notMember)
		}
	})
}

func TestGroups_Invite(t *testing.T) {
	ctx := context.Background()
	f := newGroupFixture(t)

	t.Run("メンバーでなくても名前と人数が見える", func(t *testing.T) {
		got, err := f.q.Invite(ctx, outsider, f.g.Invite().String())
		if err != nil {
			t.Fatal(err)
		}
		if got.Name != "ジム仲間" || got.MemberCount != 2 || got.AlreadyMember {
			t.Errorf("%+v。ジム仲間・2人・未参加のはず", got)
		}
	})

	t.Run("既にメンバーならそう返す", func(t *testing.T) {
		got, err := f.q.Invite(ctx, friend, f.g.Invite().String())
		if err != nil {
			t.Fatal(err)
		}
		if !got.AlreadyMember {
			t.Error("メンバーなのに AlreadyMember が false")
		}
	})

	t.Run("知らないリンクはグループが無い扱い", func(t *testing.T) {
		if _, err := f.q.Invite(ctx, outsider, "unknown"); !errors.Is(err, apperror.ErrGroupNotFound) {
			t.Errorf("%v。ErrGroupNotFound のはず", err)
		}
	})
}

func TestGroups_Mine(t *testing.T) {
	f := newGroupFixture(t)

	got, err := f.q.Mine(context.Background(), friend)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Name != "ジム仲間" || got[0].MyName != "たろう" || got[0].MemberCount != 2 {
		t.Errorf("%+v。ジム仲間（自分はたろう・2人）のはず", got)
	}
	none, err := f.q.Mine(context.Background(), outsider)
	if err != nil || len(none) != 0 {
		t.Errorf("どこにも入っていない人に %d 件 %v", len(none), err)
	}
}
