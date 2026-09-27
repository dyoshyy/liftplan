// Package memory はリポジトリのインメモリ実装。
//
// Onion の要はインフラが差し替え可能であること。ドメインが正しいことを
// DB 抜きで証明するために、まずこの実装でサーバーを動かす。
package memory

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"

	"github.com/dyoshyy/liftplan/internal/domain/account"
	"github.com/dyoshyy/liftplan/internal/domain/training/condition"
	"github.com/dyoshyy/liftplan/internal/domain/training/exercise"
	"github.com/dyoshyy/liftplan/internal/domain/training/program"
	"github.com/dyoshyy/liftplan/internal/domain/training/setlog"
)

// ExerciseRepository は種目を保持する。種目は利用者ごとの一覧で、
// プリセット（シード）はその人の行が1件も無いときに一度だけコピーする
// （global-constraints「消した行も『行がある』に数える」）。
type ExerciseRepository struct {
	mu     sync.Mutex
	seed   []*exercise.Exercise
	byUser map[account.UserID]map[exercise.ExerciseID]*exercise.Exercise
}

func NewExerciseRepository(seed []*exercise.Exercise) *ExerciseRepository {
	copied := make([]*exercise.Exercise, len(seed))
	copy(copied, seed)
	return &ExerciseRepository{
		seed:   copied,
		byUser: map[account.UserID]map[exercise.ExerciseID]*exercise.Exercise{},
	}
}

// seeded はプリセットだけを持つ新しい map を作る。呼び出し側がロックを
// 持っている前提（r.seed を読むだけで、r.byUser には触れない）。
func (r *ExerciseRepository) seeded() map[exercise.ExerciseID]*exercise.Exercise {
	m := make(map[exercise.ExerciseID]*exercise.Exercise, len(r.seed))
	for _, e := range r.seed {
		m[e.ID()] = e
	}
	return m
}

// FindAll はその利用者の一覧を ID 昇順で返す。map が無ければ（nil なら）
// プリセットを全部入れてから返す。「無ければ」の判定は map の有無であって
// 中身の件数ではない。件数で判定すると、全部消した直後（中身はあるが
// 生きている行が無い状態）にまたプリセットが入り、消した記録が生き返る。
func (r *ExerciseRepository) FindAll(
	_ context.Context, user account.UserID,
) ([]*exercise.Exercise, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	mine := r.ensureSeeded(user)

	ids := make([]exercise.ExerciseID, 0, len(mine))
	for id := range mine {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })

	out := make([]*exercise.Exercise, 0, len(mine))
	for _, id := range ids {
		out = append(out, mine[id])
	}
	return out, nil
}

// Save はその利用者の一覧に保存する。プリセット由来かどうかで扱いを
// 変えない（IsCustom は見ない）。同じ ID は上書きする。
func (r *ExerciseRepository) Save(_ context.Context, user account.UserID, e *exercise.Exercise) error {
	if e == nil {
		return errors.New("種目が nil である")
	}
	r.mu.Lock()
	defer r.mu.Unlock()

	mine := r.ensureSeeded(user)
	if !e.IsDeleted() {
		for id, other := range mine {
			if id != e.ID() && !other.IsDeleted() && other.Name() == e.Name() {
				return fmt.Errorf("%w: %s", exercise.ErrDuplicateExerciseName, e.Name())
			}
		}
	}
	mine[e.ID()] = e
	return nil
}

// ensureSeeded はその利用者の map を返す。無ければプリセットを入れて
// 作る。呼び出し側が r.mu を持っている前提。
func (r *ExerciseRepository) ensureSeeded(user account.UserID) map[exercise.ExerciseID]*exercise.Exercise {
	mine := r.byUser[user]
	if mine == nil {
		mine = r.seeded()
		r.byUser[user] = mine
	}
	return mine
}

// SetLogRepository は実績ログを「所有者とID」のキーで保持する。
// 同じキーを二度受けても重複しないため、Save は冪等になる。
//
// 利用者ごとに map を分けるのは、IDだけをキーにすると別の利用者の
// 同じIDが同じ場所に落ちるため。Postgres 側が主キーを (user_id, id) に
// しているのと同じことを、インメモリでもやる。
type SetLogRepository struct {
	mu     sync.RWMutex
	byUser map[account.UserID]map[setlog.SetLogID]*setlog.SetLog
}

func NewSetLogRepository() *SetLogRepository {
	return &SetLogRepository{
		byUser: map[account.UserID]map[setlog.SetLogID]*setlog.SetLog{},
	}
}

func (r *SetLogRepository) FindAll(
	_ context.Context, userID account.UserID,
) (setlog.History, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	byID := r.byUser[userID]

	// 取得順を安定させる。map の反復順は保証されないので、揺れると
	// History の重複解決や推定1RMの畳み込みが呼び出しごとに変わり、
	// 同じ入力から違う計画が出る。
	ids := make([]setlog.SetLogID, 0, len(byID))
	for id := range byID {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })

	out := make([]*setlog.SetLog, 0, len(ids))
	for _, id := range ids {
		out = append(out, byID[id])
	}
	return setlog.NewHistory(out), nil
}

// Save は冪等。同じ ID で同じ内容なら黙って受け入れ、内容が違えば
// ErrConflictingSetLog を返す。黙って上書きすると、リポジトリが返す順序
// 次第で推定1RMが変わり、同じ入力から違う計画が出る。
//
// 全か無かで書く。途中で衝突を見つけたら1件も書かない。半分だけ保存された
// 状態は、その週の刺激量を実態とずらしたまま計画に効き続ける。
func (r *SetLogRepository) Save(
	_ context.Context, userID account.UserID, logs []*setlog.SetLog,
) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	byID := r.byUser[userID]

	staged := make(map[setlog.SetLogID]*setlog.SetLog, len(logs))
	for i, l := range logs {
		if l == nil {
			return fmt.Errorf("%d番目のセットログが nil である", i)
		}
		// 衝突を見るのは同じ所有者の中だけ。他人の同じIDを衝突にすると、
		// 「そのIDの記録が存在する」ことが他人に分かる。
		for _, existing := range []*setlog.SetLog{byID[l.ID()], staged[l.ID()]} {
			if existing != nil && !existing.Equals(l) {
				return fmt.Errorf("%w: %s", setlog.ErrConflictingSetLog, l.ID())
			}
		}
		staged[l.ID()] = l
	}

	if byID == nil {
		byID = map[setlog.SetLogID]*setlog.SetLog{}
		r.byUser[userID] = byID
	}
	for id, l := range staged {
		byID[id] = l
	}
	return nil
}

// Delete は打ち間違いの訂正。存在しないIDでも成功として扱う。
// 所有者の map から消すので、他人の同じIDには届かない。
func (r *SetLogRepository) Delete(
	_ context.Context, userID account.UserID, id setlog.SetLogID,
) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.byUser[userID], id)
	return nil
}

// Size はその利用者の件数。テストが保存の結果を確かめるためにある。
func (r *SetLogRepository) Size(userID account.UserID) int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.byUser[userID])
}

// ConditionRepository は日次コンディションを日付キーで保持する。
// 同じ日付を二度受けたら上書きになるため、Save は冪等になる。
type ConditionRepository struct {
	mu     sync.RWMutex
	byUser map[account.UserID]map[string]condition.DailyCondition
}

func NewConditionRepository() *ConditionRepository {
	return &ConditionRepository{
		byUser: map[account.UserID]map[string]condition.DailyCondition{},
	}
}

func (r *ConditionRepository) FindAll(
	_ context.Context, userID account.UserID,
) (condition.ConditionLog, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	byDate := r.byUser[userID]

	// 並べ替えはしない。NewConditionLog が日付で整列し、同じ日付は
	// 項目ごとに合成するので、渡す順序は結果に影響しない。
	// ここで整列すると、意味のある処理に見えて実は何もしていない
	// コードが残る（実際、消しても全テストが通る状態だった）。
	out := make([]condition.DailyCondition, 0, len(byDate))
	for _, c := range byDate {
		out = append(out, c)
	}
	return condition.NewConditionLog(out), nil
}

// Save は冪等。同じ日付を二度受けたら項目ごとに上書きする。
//
// 日付ごと置き換えないのは、体重だけを送ったときに睡眠時間が消えるため。
// クライアントは体重と睡眠を別のタイミングで記録するので、これは日常的に起きる。
func (r *ConditionRepository) Save(
	_ context.Context, userID account.UserID, items []condition.DailyCondition,
) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	byDate := r.byUser[userID]

	staged := make(map[string]condition.DailyCondition, len(items))
	for i, c := range items {
		// 日付の無い記録を黙って捨てない。捨てると、クライアントは
		// 保存に成功したと思ったまま記録が消える。
		if c.Date().IsZero() {
			return fmt.Errorf("%d番目のコンディションに日付が無い", i)
		}
		key := c.Date().String()
		base, ok := staged[key]
		if !ok {
			// 合成の相手は同じ所有者の記録だけ。他人の同じ日付と
			// 混ぜると、体重も睡眠も他人の値で埋まる。
			base, ok = byDate[key]
		}
		if ok {
			c = base.Merge(c)
		}
		staged[key] = c
	}

	if byDate == nil {
		byDate = map[string]condition.DailyCondition{}
		r.byUser[userID] = byDate
	}
	for k, c := range staged {
		byDate[k] = c
	}
	return nil
}

// Size はその利用者の件数。
func (r *ConditionRepository) Size(userID account.UserID) int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.byUser[userID])
}

// ProgramRepository は利用者ごとに設定を1つ保持する。
//
// 初期値を受け取る形をやめた。初期プログラムを入れるのは「起動時に1人分」
// ではなく「その利用者が最初に来たとき」の話になり、決める場所は配線層
// （いまは cmd、OAuth が入れば初回ログインの受け入れ）に移る。
type ProgramRepository struct {
	mu     sync.RWMutex
	byUser map[account.UserID]*program.Program
}

func NewProgramRepository() *ProgramRepository {
	return &ProgramRepository{byUser: map[account.UserID]*program.Program{}}
}

func (r *ProgramRepository) Get(
	_ context.Context, userID account.UserID,
) (*program.Program, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	p, ok := r.byUser[userID]
	if !ok || p == nil {
		return nil, program.ErrProgramNotConfigured
	}
	return p, nil
}

// Save は冪等。プログラムはユーザーごとに1つで、保存は常に全体の置き換え。
func (r *ProgramRepository) Save(
	_ context.Context, userID account.UserID, p *program.Program,
) error {
	if p == nil {
		return fmt.Errorf("プログラムが nil である")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.byUser[userID] = p
	return nil
}
