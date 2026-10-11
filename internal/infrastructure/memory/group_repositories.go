package memory

import (
	"cmp"
	"context"
	"fmt"
	"slices"
	"strings"
	"sync"

	"github.com/dyoshyy/liftplan/internal/domain/account"
	"github.com/dyoshyy/liftplan/internal/domain/group"
	"github.com/dyoshyy/liftplan/internal/domain/training"
	"github.com/dyoshyy/liftplan/internal/domain/training/exercise"
	"github.com/dyoshyy/liftplan/internal/domain/training/setlog"
)

// GroupRepository はグループを ID で保持する。
//
// 持つのは組み立てる材料（group.Params）で、*group.Group そのものではない。
// 返した *Group を呼び出し側が Join や Leave で書き換えても、保存した中身が
// 変わらないようにするため。Postgres が行から組み直すのと同じ振る舞いになる。
type GroupRepository struct {
	mu   sync.RWMutex
	byID map[group.ID]group.Params
}

func NewGroupRepository() *GroupRepository {
	return &GroupRepository{byID: map[group.ID]group.Params{}}
}

// Find はそのグループを返す。
func (r *GroupRepository) Find(_ context.Context, id group.ID) (*group.Group, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	p, ok := r.byID[id]
	if !ok {
		return nil, fmt.Errorf("%w: %s", group.ErrGroupNotFound, id)
	}
	return group.New(p)
}

// FindByInvite はそのトークンのグループを返す。索引は持たず、毎回なめる。
func (r *GroupRepository) FindByInvite(_ context.Context, token group.InviteToken) (*group.Group, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, p := range r.byID {
		if p.Invite == token {
			return group.New(p)
		}
	}
	return nil, fmt.Errorf("%w: 招待トークンに当たらない", group.ErrGroupNotFound)
}

// ListByMember はその人がメンバーのグループを作った順に返す。
func (r *GroupRepository) ListByMember(_ context.Context, user account.UserID) ([]*group.Group, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var ps []group.Params
	for _, p := range r.byID {
		if slices.ContainsFunc(p.Members, func(m group.Member) bool { return m.UserID() == user }) {
			ps = append(ps, p)
		}
	}
	// 同時刻は ID で決める。map の走査順で並びが変わらないように。
	slices.SortFunc(ps, func(a, b group.Params) int {
		return cmp.Or(a.CreatedAt.Compare(b.CreatedAt), strings.Compare(a.ID.String(), b.ID.String()))
	})

	out := make([]*group.Group, 0, len(ps))
	for _, p := range ps {
		g, err := group.New(p)
		if err != nil {
			return nil, err
		}
		out = append(out, g)
	}
	return out, nil
}

// Create はグループを最初のメンバーごと保存する。
func (r *GroupRepository) Create(_ context.Context, g *group.Group) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.byID[g.ID()]; ok {
		return fmt.Errorf("同じ ID のグループが既にある: %s", g.ID())
	}
	r.byID[g.ID()] = group.Params{
		ID: g.ID(), Name: g.Name(), Invite: g.Invite(), Members: g.Members(), CreatedAt: g.CreatedAt(),
	}
	return nil
}

// AddMember はメンバーを足す。既にメンバーなら何もしない。
func (r *GroupRepository) AddMember(_ context.Context, id group.ID, m group.Member) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	p, ok := r.byID[id]
	if !ok {
		return fmt.Errorf("%w: %s", group.ErrGroupNotFound, id)
	}
	if slices.ContainsFunc(p.Members, func(x group.Member) bool { return x.UserID() == m.UserID() }) {
		return nil
	}
	p.Members = append(slices.Clone(p.Members), m)
	r.byID[id] = p
	return nil
}

// RemoveMember はメンバーを外す。いなければ何もしない。
func (r *GroupRepository) RemoveMember(_ context.Context, id group.ID, user account.UserID) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	p, ok := r.byID[id]
	if !ok {
		return nil
	}
	p.Members = slices.DeleteFunc(slices.Clone(p.Members), func(m group.Member) bool { return m.UserID() == user })
	r.byID[id] = p
	return nil
}

// ReplaceInvite は招待トークンを差し替える。
func (r *GroupRepository) ReplaceInvite(_ context.Context, id group.ID, token group.InviteToken) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	p, ok := r.byID[id]
	if !ok {
		return fmt.Errorf("%w: %s", group.ErrGroupNotFound, id)
	}
	p.Invite = token
	r.byID[id] = p
	return nil
}

// DeleteIfEmpty はメンバーがいなければグループを消す。
func (r *GroupRepository) DeleteIfEmpty(_ context.Context, id group.ID) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if p, ok := r.byID[id]; ok && len(p.Members) == 0 {
		delete(r.byID, id)
	}
	return nil
}

// GroupActivityReader は仲間の記録を、記録と種目のリポジトリから組み立てる。
//
// Postgres は1本の SQL で引くが、インメモリには結合が無いので、
// Audience の1人ずつ自分の口（所有者で絞った FindAll）を呼ぶ。
// 他人の記録を読むのは、Audience にいる人の分だけ。
type GroupActivityReader struct {
	logs      setlog.Reader
	exercises exercise.Reader
}

func NewGroupActivityReader(logs setlog.Reader, exercises exercise.Reader) *GroupActivityReader {
	return &GroupActivityReader{logs: logs, exercises: exercises}
}

// Activity は Audience の人の from〜to の記録を (人, 日, 種目名) で返す。
func (r *GroupActivityReader) Activity(
	ctx context.Context, a group.Audience, from, to training.Date,
) ([]group.Activity, error) {
	var out []group.Activity
	for _, user := range a.UserIDs() {
		names, err := r.namesOf(ctx, user)
		if err != nil {
			return nil, err
		}
		h, err := r.logs.FindAll(ctx, user)
		if err != nil {
			return nil, err
		}

		// 重複は名前で消す（契約）。ID で消すと、消した種目と同じ名前で
		// 作り直した種目を同じ日にやったとき、同じ名前が2件並ぶ。
		type key struct {
			date training.Date
			name string
		}
		seen := map[key]bool{}
		for _, l := range h.Logs() {
			d := l.PerformedOn()
			if d.Before(from) || d.After(to) {
				continue
			}
			k := key{date: d, name: names[l.ExerciseID()]}
			if seen[k] {
				continue
			}
			seen[k] = true
			out = append(out, group.NewActivity(user, d, k.name))
		}
	}
	return out, nil
}

// namesOf はその人の種目 ID から名前への対応を返す。消した種目も含む。
func (r *GroupActivityReader) namesOf(ctx context.Context, user account.UserID) (map[exercise.ExerciseID]string, error) {
	all, err := r.exercises.FindAll(ctx, user)
	if err != nil {
		return nil, err
	}
	out := make(map[exercise.ExerciseID]string, len(all))
	for _, e := range all {
		out[e.ID()] = e.Name()
	}
	return out, nil
}
