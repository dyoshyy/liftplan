package group

import (
	"context"

	"github.com/dyoshyy/liftplan/internal/domain/account"
)

// Reader はグループの読み出し口。
type Reader interface {
	// Find はそのグループを返す。無ければ ErrGroupNotFound。
	Find(ctx context.Context, id ID) (*Group, error)
	// FindByInvite はそのトークンのグループを返す。作り直して使えなく
	// なったトークンも含め、引けなければ ErrGroupNotFound。
	FindByInvite(ctx context.Context, token InviteToken) (*Group, error)
	// ListByMember はその人がメンバーのグループを作った順に返す。
	// 無ければ空のスライス（エラーではない）。
	ListByMember(ctx context.Context, user account.UserID) ([]*Group, error)
}

// Writer はグループの書き込み口。
//
// グループを丸ごと保存する Save にしないのは、出入りが同時に起きるから。
// 2人が同時に参加して、それぞれが「自分を足したメンバー一覧」で上書き
// すると、先に入った1人が消える。変更を1件ずつ渡せば、互いに踏まない。
type Writer interface {
	// Create はグループを最初のメンバーごと保存する。
	Create(ctx context.Context, g *Group) error
	// AddMember はメンバーを足す。既にメンバーなら何もしない（成功）。
	// グループが無ければ ErrGroupNotFound。
	AddMember(ctx context.Context, id ID, m Member) error
	// RemoveMember はメンバーを外す。いなければ何もしない（成功）。
	RemoveMember(ctx context.Context, id ID, user account.UserID) error
	// ReplaceInvite は招待トークンを差し替える。グループが無ければ ErrGroupNotFound。
	ReplaceInvite(ctx context.Context, id ID, token InviteToken) error
	// DeleteIfEmpty はメンバーがいなければグループを消す。メンバーが
	// いれば何もしない（成功）。グループが無くても成功。
	//
	// 消すと決めるのはドメイン（Group.IsEmpty）で、ここは消す瞬間にまだ
	// 空かだけを確かめる。最後の1人が抜けた判断と消す処理の間に誰かが
	// 参加したとき、その人ごと消さないため。
	DeleteIfEmpty(ctx context.Context, id ID) error
}
