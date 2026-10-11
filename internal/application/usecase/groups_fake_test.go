package usecase_test

import (
	"context"
	"fmt"
	"slices"
	"sync"
	"testing"

	"github.com/dyoshyy/liftplan/internal/domain/account"
	"github.com/dyoshyy/liftplan/internal/domain/group"
	"github.com/dyoshyy/liftplan/internal/domain/group/grouptest"
)

// fakeGroups はこのパッケージのテストが使うグループの保存先。
//
// application は infrastructure（memory）を import できず
// （internal/architecture_test.go）、ドメイン層には書き換える型を置けない
// （training/architecture_test.go）ので、テストの中に最小の実装を置く。
// 本物と同じ契約を満たすことは TestFakeGroups_Contract が確かめる。
type fakeGroups struct {
	mu   sync.Mutex
	byID map[group.ID]group.Params
}

func newFakeGroups() *fakeGroups { return &fakeGroups{byID: map[group.ID]group.Params{}} }

func (s *fakeGroups) Find(_ context.Context, id group.ID) (*group.Group, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, ok := s.byID[id]
	if !ok {
		return nil, fmt.Errorf("%w: %s", group.ErrGroupNotFound, id)
	}
	return group.New(p)
}

func (s *fakeGroups) FindByInvite(_ context.Context, token group.InviteToken) (*group.Group, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, p := range s.byID {
		if p.Invite == token {
			return group.New(p)
		}
	}
	return nil, group.ErrGroupNotFound
}

func (s *fakeGroups) ListByMember(_ context.Context, user account.UserID) ([]*group.Group, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var ps []group.Params
	for _, p := range s.byID {
		if slices.ContainsFunc(p.Members, func(m group.Member) bool { return m.UserID() == user }) {
			ps = append(ps, p)
		}
	}
	slices.SortFunc(ps, func(a, b group.Params) int { return a.CreatedAt.Compare(b.CreatedAt) })
	out := []*group.Group{}
	for _, p := range ps {
		g, err := group.New(p)
		if err != nil {
			return nil, err
		}
		out = append(out, g)
	}
	return out, nil
}

func (s *fakeGroups) Create(_ context.Context, g *group.Group) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.byID[g.ID()] = group.Params{ID: g.ID(), Name: g.Name(), Invite: g.Invite(), Members: g.Members(), CreatedAt: g.CreatedAt()}
	return nil
}

func (s *fakeGroups) AddMember(_ context.Context, id group.ID, m group.Member) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, ok := s.byID[id]
	if !ok {
		return group.ErrGroupNotFound
	}
	if !slices.ContainsFunc(p.Members, func(x group.Member) bool { return x.UserID() == m.UserID() }) {
		p.Members = append(slices.Clone(p.Members), m)
	}
	s.byID[id] = p
	return nil
}

func (s *fakeGroups) RemoveMember(_ context.Context, id group.ID, user account.UserID) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if p, ok := s.byID[id]; ok {
		p.Members = slices.DeleteFunc(slices.Clone(p.Members), func(m group.Member) bool { return m.UserID() == user })
		s.byID[id] = p
	}
	return nil
}

func (s *fakeGroups) ReplaceInvite(_ context.Context, id group.ID, token group.InviteToken) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, ok := s.byID[id]
	if !ok {
		return group.ErrGroupNotFound
	}
	p.Invite = token
	s.byID[id] = p
	return nil
}

func (s *fakeGroups) DeleteIfEmpty(_ context.Context, id group.ID) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if p, ok := s.byID[id]; ok && len(p.Members) == 0 {
		delete(s.byID, id)
	}
	return nil
}

// 偽物が本物と違う振る舞いをすると、ユースケースのテストが本番では
// 起きない前提の上で緑になる。
func TestFakeGroups_Contract(t *testing.T) {
	grouptest.RunGroupContract(t, func(t *testing.T) grouptest.Repos {
		return grouptest.Repos{Groups: newFakeGroups()}
	})
}
