package group

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/dyoshyy/liftplan/internal/domain/account"
)

var (
	// ErrInvalidGroup はグループとして成立しない値を表す。
	ErrInvalidGroup = errors.New("グループの内容が不正である")

	// ErrGroupNotFound はそのグループが無いことを表す。
	//
	// 無効になった招待トークンで引いたときもこれを返す。「前はあった」を
	// 区別して返すと、当てずっぽうに対して答えを変えることになる。
	ErrGroupNotFound = errors.New("グループが見つからない")

	// ErrNotMember はその人がメンバーでないことを表す。
	//
	// 上の層では ErrGroupNotFound と同じ 404 に写す。403 にすると
	// 「そのグループは存在する」ことが分かる。
	ErrNotMember = errors.New("グループのメンバーではない")
)

// Member はグループの1人。
type Member struct {
	userID      account.UserID
	displayName DisplayName
	joinedAt    time.Time
}

// NewMember はメンバーを組み立てる。
func NewMember(user account.UserID, name DisplayName, joinedAt time.Time) (Member, error) {
	switch {
	case user == (account.UserID{}):
		return Member{}, fmt.Errorf("%w: 利用者が無い", ErrInvalidGroup)
	case name == (DisplayName{}):
		return Member{}, fmt.Errorf("%w: 表示名が無い", ErrInvalidGroup)
	case joinedAt.IsZero():
		return Member{}, fmt.Errorf("%w: 参加した時刻が無い", ErrInvalidGroup)
	}
	return Member{userID: user, displayName: name, joinedAt: joinedAt.UTC()}, nil
}

func (m Member) UserID() account.UserID   { return m.userID }
func (m Member) DisplayName() DisplayName { return m.displayName }
func (m Member) JoinedAt() time.Time      { return m.joinedAt }

// Group は記録を見せ合う仲間の集まり。
//
// 作ったあとは変わらない。メンバーの出入りと招待の作り直しは、それぞれ
// 新しい Group を返す（program.Program の With* と同じ）。変わった部分を
// リポジトリの口（Writer）へ渡すのはユースケースの仕事。
type Group struct {
	id        ID
	name      Name
	invite    InviteToken
	members   []Member
	createdAt time.Time
}

// Params はグループを組み立てる材料。新しく作るときも、保存先から
// 読み戻すときも同じ口を通す。
type Params struct {
	ID        ID
	Name      Name
	Invite    InviteToken
	Members   []Member
	CreatedAt time.Time
}

// New はグループを組み立てる。
//
// メンバーが0人のグループも組み立てられる。最後の1人が抜けた直後の
// 状態で、ユースケースはそれを見て消す（IsEmpty）。2人が同時に抜けると
// 空のまま保存先に残りうるので、読み戻しでも弾かない（仕様「最後の1人が
// 抜けたとき」）。
func New(p Params) (*Group, error) {
	switch {
	case p.ID.IsZero():
		return nil, fmt.Errorf("%w: ID が無い", ErrInvalidGroup)
	case p.Name == (Name{}):
		return nil, fmt.Errorf("%w: 名前が無い", ErrInvalidGroup)
	case p.Invite == (InviteToken{}):
		return nil, fmt.Errorf("%w: 招待トークンが無い", ErrInvalidGroup)
	case p.CreatedAt.IsZero():
		return nil, fmt.Errorf("%w: 作った時刻が無い", ErrInvalidGroup)
	}

	seen := make(map[account.UserID]bool, len(p.Members))
	for _, m := range p.Members {
		if m == (Member{}) {
			return nil, fmt.Errorf("%w: ゼロ値のメンバーがいる", ErrInvalidGroup)
		}
		if seen[m.userID] {
			return nil, fmt.Errorf("%w: 同じ人が二度いる: %s", ErrInvalidGroup, m.userID)
		}
		seen[m.userID] = true
	}

	members := slices.Clone(p.Members)
	sortByJoined(members)
	return &Group{
		id: p.ID, name: p.Name, invite: p.Invite, members: members, createdAt: p.CreatedAt.UTC(),
	}, nil
}

// sortByJoined は参加順に並べる。同時刻は利用者の識別子で決める。
//
// 同時刻を決めておかないと、保存先の返す順で画面の行が入れ替わる。
func sortByJoined(ms []Member) {
	slices.SortStableFunc(ms, func(a, b Member) int {
		if c := a.joinedAt.Compare(b.joinedAt); c != 0 {
			return c
		}
		return strings.Compare(a.userID.String(), b.userID.String())
	})
}

func (g *Group) ID() ID               { return g.id }
func (g *Group) Name() Name           { return g.name }
func (g *Group) Invite() InviteToken  { return g.invite }
func (g *Group) CreatedAt() time.Time { return g.createdAt }

// Members はメンバーを参加順に返す。
func (g *Group) Members() []Member { return slices.Clone(g.members) }

// IsEmpty はメンバーが1人もいないか。
func (g *Group) IsEmpty() bool { return len(g.members) == 0 }

// IsMember はその人がメンバーか。
func (g *Group) IsMember(user account.UserID) bool {
	_, ok := g.memberOf(user)
	return ok
}

// memberOf はその人のメンバーの行を返す。
func (g *Group) memberOf(user account.UserID) (Member, bool) {
	i := slices.IndexFunc(g.members, func(m Member) bool { return m.userID == user })
	if i < 0 {
		return Member{}, false
	}
	return g.members[i], true
}

// Join はその人を加えたグループと、その人のメンバーの行と、新しく
// 加わったかを返す。
//
// 既にメンバーなら同じグループと既存の行を返し、false を返す。表示名も
// 書き換えない。参加の二度押しや通信のやり直しをエラーにしないため（冪等）。
func (g *Group) Join(user account.UserID, name DisplayName, now time.Time) (*Group, Member, bool, error) {
	if m, ok := g.memberOf(user); ok {
		return g, m, false, nil
	}
	m, err := NewMember(user, name, now)
	if err != nil {
		return nil, Member{}, false, err
	}
	members := append(slices.Clone(g.members), m)
	sortByJoined(members)
	return g.with(members, g.invite), m, true, nil
}

// Leave はその人を外したグループを返す。メンバーでなければ ErrNotMember。
func (g *Group) Leave(user account.UserID) (*Group, error) {
	if !g.IsMember(user) {
		return nil, fmt.Errorf("%w: %s", ErrNotMember, user)
	}
	members := slices.DeleteFunc(slices.Clone(g.members), func(m Member) bool { return m.userID == user })
	return g.with(members, g.invite), nil
}

// RotateInvite は招待トークンを next に替えたグループを返す。古いトークンは
// 使えなくなる。
//
// メンバーなら誰でもできる（管理者は置かない）。メンバーでなければ ErrNotMember。
func (g *Group) RotateInvite(actor account.UserID, next InviteToken) (*Group, error) {
	if !g.IsMember(actor) {
		return nil, fmt.Errorf("%w: %s", ErrNotMember, actor)
	}
	if next == (InviteToken{}) {
		return nil, fmt.Errorf("%w: 新しい招待トークンが無い", ErrInvalidGroup)
	}
	return g.with(g.members, next), nil
}

// with はメンバーと招待トークンだけを差し替えた新しいグループを返す。
// members はここで所有権を受け取る（呼び出し側は複製してから渡す）。
func (g *Group) with(members []Member, invite InviteToken) *Group {
	return &Group{id: g.id, name: g.name, invite: invite, members: members, createdAt: g.createdAt}
}

// VisibleTo は viewer に記録を見せてよいメンバーを返す。
//
// 他人の記録を見てよいかを決めるのはここだけ。viewer がメンバーでなければ
// ErrNotMember。
//
// 見せるのは、いまのメンバー全員（viewer 自身を含む）。抜けた人は
// members にいないので、ここで自然に外れる。参加より前の記録を見せるか
// どうかもここで決まる規則だが、いまは絞らない（仕様「決めたこと」）。
func (g *Group) VisibleTo(viewer account.UserID) (Audience, error) {
	if !g.IsMember(viewer) {
		return Audience{}, fmt.Errorf("%w: %s", ErrNotMember, viewer)
	}
	return Audience{members: slices.Clone(g.members)}, nil
}

// Audience は「この人たちの記録を viewer に見せてよい」という判断の結果。
//
// 非公開フィールドだけを持つので、中身のある値は VisibleTo からしか
// 作れない。ゼロ値（Audience{}）は外から書けるが、誰も含まないので
// それで読んでも何も返らない。
type Audience struct {
	members []Member
}

// Members は見せてよいメンバーを参加順に返す。
func (a Audience) Members() []Member { return slices.Clone(a.members) }

// UserIDs は見せてよい利用者の識別子を返す。読み口が絞り込みに使う。
func (a Audience) UserIDs() []account.UserID {
	out := make([]account.UserID, 0, len(a.members))
	for _, m := range a.members {
		out = append(out, m.userID)
	}
	return out
}
