package usecase_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/dyoshyy/liftplan/internal/application/apperror"
	"github.com/dyoshyy/liftplan/internal/application/usecase"
	"github.com/dyoshyy/liftplan/internal/domain/group"
)

var (
	groupNow = time.Date(2026, 10, 11, 12, 0, 0, 0, time.UTC)
	otherA   = mustTestUserID("22222222-2222-4222-8222-222222222222")
	outsider = mustTestUserID("33333333-3333-4333-8333-333333333333")
)

func clock() time.Time { return groupNow }

// createdGroup は testUser が作ったグループを返す。
func createdGroup(t *testing.T, repo *fakeGroups) *group.Group {
	t.Helper()
	g, err := usecase.NewCreateGroup(repo, clock).Execute(context.Background(), testUser,
		usecase.CreateGroupInput{Name: "ジム仲間", DisplayName: "よし"})
	if err != nil {
		t.Fatalf("作れない: %v", err)
	}
	return g
}

func members(t *testing.T, repo *fakeGroups, id group.ID) []group.Member {
	t.Helper()
	g, err := repo.Find(context.Background(), id)
	if err != nil {
		t.Fatalf("引けない: %v", err)
	}
	return g.Members()
}

func TestCreateGroup(t *testing.T) {
	t.Run("作った人が最初のメンバーになる", func(t *testing.T) {
		repo := newFakeGroups()
		g := createdGroup(t, repo)

		ms := members(t, repo, g.ID())
		if len(ms) != 1 || ms[0].UserID() != testUser || ms[0].DisplayName().String() != "よし" {
			t.Errorf("メンバーが %v。作った人1人のはず", ms)
		}
	})

	t.Run("名前が空なら入力の不正", func(t *testing.T) {
		_, err := usecase.NewCreateGroup(newFakeGroups(), clock).Execute(context.Background(), testUser,
			usecase.CreateGroupInput{Name: " ", DisplayName: "よし"})
		if !errors.Is(err, apperror.ErrInvalidInput) {
			t.Errorf("%v。ErrInvalidInput のはず", err)
		}
	})
}

func TestJoinGroup(t *testing.T) {
	ctx := context.Background()

	t.Run("リンクの人が加わり、二度押しても1人で表示名は最初のまま", func(t *testing.T) {
		repo := newFakeGroups()
		g := createdGroup(t, repo)
		join := usecase.NewJoinGroup(repo, clock)

		for i, name := range []string{"たろう", "別名"} {
			id, err := join.Execute(ctx, otherA, g.Invite().String(), name)
			if err != nil {
				t.Fatalf("%d回目: %v", i+1, err)
			}
			if id != g.ID() {
				t.Errorf("%d回目: 返した ID が %s。%s のはず", i+1, id, g.ID())
			}
		}
		ms := members(t, repo, g.ID())
		if len(ms) != 2 || ms[1].UserID() != otherA || ms[1].DisplayName().String() != "たろう" {
			t.Errorf("メンバーが %v。[よし たろう] のはず", ms)
		}
	})

	// 1回目が通ったあとの2回目が名前の検証で落ちると、参加できたことが画面から分からない。
	t.Run("既にメンバーなら表示名が空でも成功する", func(t *testing.T) {
		repo := newFakeGroups()
		g := createdGroup(t, repo)
		if _, err := usecase.NewJoinGroup(repo, clock).Execute(ctx, testUser, g.Invite().String(), ""); err != nil {
			t.Errorf("失敗した: %v", err)
		}
	})

	t.Run("知らないリンクはグループが無い扱い", func(t *testing.T) {
		repo := newFakeGroups()
		createdGroup(t, repo)
		_, err := usecase.NewJoinGroup(repo, clock).Execute(ctx, otherA, "unknown-token", "たろう")
		if !errors.Is(err, apperror.ErrGroupNotFound) {
			t.Errorf("%v。ErrGroupNotFound のはず", err)
		}
	})
}

func TestLeaveGroup(t *testing.T) {
	ctx := context.Background()

	t.Run("抜けた人だけが消え、グループは残る", func(t *testing.T) {
		repo := newFakeGroups()
		g := createdGroup(t, repo)
		if _, err := usecase.NewJoinGroup(repo, clock).Execute(ctx, otherA, g.Invite().String(), "たろう"); err != nil {
			t.Fatal(err)
		}
		if err := usecase.NewLeaveGroup(repo).Execute(ctx, testUser, g.ID().String()); err != nil {
			t.Fatalf("抜けられない: %v", err)
		}
		ms := members(t, repo, g.ID())
		if len(ms) != 1 || ms[0].UserID() != otherA {
			t.Errorf("メンバーが %v。たろう1人のはず", ms)
		}
	})

	// 残すと、誰にも見えないグループの招待リンクが生き続ける。
	t.Run("最後の1人が抜けたらグループごと消える", func(t *testing.T) {
		repo := newFakeGroups()
		g := createdGroup(t, repo)
		if err := usecase.NewLeaveGroup(repo).Execute(ctx, testUser, g.ID().String()); err != nil {
			t.Fatalf("抜けられない: %v", err)
		}
		if _, err := repo.Find(ctx, g.ID()); !errors.Is(err, group.ErrGroupNotFound) {
			t.Errorf("%v。消えているはず", err)
		}
	})

	t.Run("メンバーでない人はグループが無い扱い", func(t *testing.T) {
		repo := newFakeGroups()
		g := createdGroup(t, repo)
		err := usecase.NewLeaveGroup(repo).Execute(ctx, outsider, g.ID().String())
		if !errors.Is(err, apperror.ErrGroupNotFound) {
			t.Errorf("%v。ErrGroupNotFound のはず", err)
		}
	})
}

func TestRotateInvite(t *testing.T) {
	ctx := context.Background()

	t.Run("作り直すと古いリンクでは入れず、新しいリンクで入れる", func(t *testing.T) {
		repo := newFakeGroups()
		g := createdGroup(t, repo)
		next, err := usecase.NewRotateInvite(repo).Execute(ctx, testUser, g.ID().String())
		if err != nil {
			t.Fatalf("作り直せない: %v", err)
		}
		join := usecase.NewJoinGroup(repo, clock)
		if _, err := join.Execute(ctx, otherA, g.Invite().String(), "たろう"); !errors.Is(err, apperror.ErrGroupNotFound) {
			t.Errorf("古いリンクで: %v。ErrGroupNotFound のはず", err)
		}
		if _, err := join.Execute(ctx, otherA, next.String(), "たろう"); err != nil {
			t.Errorf("新しいリンクで入れない: %v", err)
		}
	})

	// 外の人が作り直せると、メンバーの持っているリンクを無効にできる。
	t.Run("メンバーでない人はグループが無い扱い", func(t *testing.T) {
		repo := newFakeGroups()
		g := createdGroup(t, repo)
		if _, err := usecase.NewRotateInvite(repo).Execute(ctx, outsider, g.ID().String()); !errors.Is(err, apperror.ErrGroupNotFound) {
			t.Errorf("%v。ErrGroupNotFound のはず", err)
		}
		got, _ := repo.Find(ctx, g.ID())
		if got.Invite() != g.Invite() {
			t.Error("弾いたのにリンクが替わった")
		}
	})
}
