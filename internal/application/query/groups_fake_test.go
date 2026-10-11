package query_test

import (
	"context"
	"fmt"
	"slices"

	"github.com/dyoshyy/liftplan/internal/domain/account"
	"github.com/dyoshyy/liftplan/internal/domain/group"
	"github.com/dyoshyy/liftplan/internal/domain/training"
)

// fakeGroups は決まったグループだけを返す読み口。
//
// application は infrastructure（memory）を import できない
// （internal/architecture_test.go）ので、テストの中に置く。
type fakeGroups struct {
	all []*group.Group
}

func (f fakeGroups) Find(_ context.Context, id group.ID) (*group.Group, error) {
	for _, g := range f.all {
		if g.ID() == id {
			return g, nil
		}
	}
	return nil, fmt.Errorf("%w: %s", group.ErrGroupNotFound, id)
}

func (f fakeGroups) FindByInvite(_ context.Context, token group.InviteToken) (*group.Group, error) {
	for _, g := range f.all {
		if g.Invite() == token {
			return g, nil
		}
	}
	return nil, group.ErrGroupNotFound
}

func (f fakeGroups) ListByMember(_ context.Context, user account.UserID) ([]*group.Group, error) {
	out := []*group.Group{}
	for _, g := range f.all {
		if g.IsMember(user) {
			out = append(out, g)
		}
	}
	return out, nil
}

// fakeActivity は渡された記録を、Audience と期間で絞って返す読み口。
//
// 種目名の引き方（持ち主の一覧から）は本物の実装の契約で、ここは
// 名前を入れた Activity をそのまま持つ。
type fakeActivity struct {
	All []group.Activity
}

func (f *fakeActivity) Activity(_ context.Context, a group.Audience, from, to training.Date) ([]group.Activity, error) {
	users := a.UserIDs()
	out := []group.Activity{}
	for _, x := range f.All {
		if slices.Contains(users, x.UserID()) && !x.Date().Before(from) && !x.Date().After(to) {
			out = append(out, x)
		}
	}
	return out, nil
}
