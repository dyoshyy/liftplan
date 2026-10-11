package usecase

import (
	"context"
	"fmt"
	"time"

	"github.com/dyoshyy/liftplan/internal/application/apperror"
	"github.com/dyoshyy/liftplan/internal/domain/account"
	"github.com/dyoshyy/liftplan/internal/domain/group"
)

// グループの書き込み4本（作る・参加する・抜ける・リンクを作り直す）。
//
// 1ファイルにまとめるのは、どれも同じ形（引いて、ドメインに判断させて、
// 変わった1件を書く）で、並べて読むほうが違いが見えるため。

// groupStore はグループの読み書き。作る以外の3本が両方を要る。
type groupStore interface {
	group.Reader
	group.Writer
}

// CreateGroup はグループを作り、作った人を最初のメンバーにする。
type CreateGroup struct {
	groups group.Writer
	now    func() time.Time
}

func NewCreateGroup(groups group.Writer, now func() time.Time) *CreateGroup {
	return &CreateGroup{groups: groups, now: now}
}

// CreateGroupInput は作るときの入力。
type CreateGroupInput struct {
	Name        string
	DisplayName string
}

// Execute はグループを作って返す。
func (u *CreateGroup) Execute(ctx context.Context, user account.UserID, in CreateGroupInput) (_ *group.Group, err error) {
	defer func() { err = apperror.Classify(err) }()

	name, err := group.NewName(in.Name)
	if err != nil {
		return nil, err
	}
	display, err := group.NewDisplayName(in.DisplayName)
	if err != nil {
		return nil, err
	}

	now := u.now()
	creator, err := group.NewMember(user, display, now)
	if err != nil {
		return nil, err
	}
	id, err := group.NewRandomID()
	if err != nil {
		return nil, err
	}
	token, err := group.NewInviteToken()
	if err != nil {
		return nil, err
	}
	g, err := group.New(group.Params{
		ID: id, Name: name, Invite: token, Members: []group.Member{creator}, CreatedAt: now,
	})
	if err != nil {
		return nil, err
	}

	if err := u.groups.Create(ctx, g); err != nil {
		return nil, fmt.Errorf("グループを保存できない: %w", err)
	}
	return g, nil
}

// JoinGroup は招待トークンのグループに参加する。
type JoinGroup struct {
	groups groupStore
	now    func() time.Time
}

func NewJoinGroup(groups groupStore, now func() time.Time) *JoinGroup {
	return &JoinGroup{groups: groups, now: now}
}

// Execute は参加したグループの ID を返す。
//
// 既にメンバーなら何も変えずに同じ ID を返す（冪等）。表示名の検証も
// しない。二度押しで2回目だけ名前の不正で失敗すると、1回目が成功した
// ことが画面から分からなくなる。
func (u *JoinGroup) Execute(
	ctx context.Context, user account.UserID, rawToken, displayName string,
) (_ group.ID, err error) {
	defer func() { err = apperror.Classify(err) }()

	token, err := group.ParseInviteToken(rawToken)
	if err != nil {
		return group.ID{}, err
	}
	g, err := u.groups.FindByInvite(ctx, token)
	if err != nil {
		return group.ID{}, err
	}
	if g.IsMember(user) {
		return g.ID(), nil
	}

	name, err := group.NewDisplayName(displayName)
	if err != nil {
		return group.ID{}, err
	}
	_, m, _, err := g.Join(user, name, u.now())
	if err != nil {
		return group.ID{}, err
	}
	if err := u.groups.AddMember(ctx, g.ID(), m); err != nil {
		return group.ID{}, fmt.Errorf("参加を保存できない: %w", err)
	}
	return g.ID(), nil
}

// LeaveGroup はグループから抜ける。最後の1人ならグループごと消す。
type LeaveGroup struct {
	groups groupStore
}

func NewLeaveGroup(groups groupStore) *LeaveGroup {
	return &LeaveGroup{groups: groups}
}

// Execute は user をそのグループから外す。
func (u *LeaveGroup) Execute(ctx context.Context, user account.UserID, rawID string) (err error) {
	defer func() { err = apperror.Classify(err) }()

	id, err := group.ParseID(rawID)
	if err != nil {
		return err
	}
	g, err := u.groups.Find(ctx, id)
	if err != nil {
		return err
	}
	next, err := g.Leave(user)
	if err != nil {
		return err
	}

	if err := u.groups.RemoveMember(ctx, id, user); err != nil {
		return fmt.Errorf("退出を保存できない: %w", err)
	}
	// 空になったかはドメインが決め、保存先は消す瞬間にまだ空かだけを見る
	// （DeleteIfEmpty）。その間に誰かが参加していれば消えない。
	if next.IsEmpty() {
		if err := u.groups.DeleteIfEmpty(ctx, id); err != nil {
			return fmt.Errorf("空のグループを消せない: %w", err)
		}
	}
	return nil
}

// RotateInvite は招待リンクを作り直す。古いリンクは使えなくなる。
type RotateInvite struct {
	groups groupStore
}

func NewRotateInvite(groups groupStore) *RotateInvite {
	return &RotateInvite{groups: groups}
}

// Execute は新しい招待トークンを返す。
func (u *RotateInvite) Execute(ctx context.Context, user account.UserID, rawID string) (_ group.InviteToken, err error) {
	defer func() { err = apperror.Classify(err) }()

	id, err := group.ParseID(rawID)
	if err != nil {
		return group.InviteToken{}, err
	}
	g, err := u.groups.Find(ctx, id)
	if err != nil {
		return group.InviteToken{}, err
	}
	token, err := group.NewInviteToken()
	if err != nil {
		return group.InviteToken{}, err
	}
	next, err := g.RotateInvite(user, token)
	if err != nil {
		return group.InviteToken{}, err
	}
	if err := u.groups.ReplaceInvite(ctx, id, next.Invite()); err != nil {
		return group.InviteToken{}, fmt.Errorf("招待リンクを保存できない: %w", err)
	}
	return next.Invite(), nil
}
