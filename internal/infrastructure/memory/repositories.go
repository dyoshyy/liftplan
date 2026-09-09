// Package memory はリポジトリのインメモリ実装。
//
// Onion の要はインフラが差し替え可能であること。ドメインが正しいことを
// DB 抜きで証明するために、まずこの実装でサーバーを動かす。
package memory

import (
	"context"
	"fmt"
	"sort"
	"sync"

	"github.com/dyoshyy/liftplan/internal/domain/training/condition"
	"github.com/dyoshyy/liftplan/internal/domain/training/exercise"
	"github.com/dyoshyy/liftplan/internal/domain/training/program"
	"github.com/dyoshyy/liftplan/internal/domain/training/setlog"
)

// ExerciseRepository は種目マスタを保持する。起動時にシードを流し込む。
type ExerciseRepository struct {
	mu  sync.RWMutex
	all []*exercise.Exercise
}

func NewExerciseRepository(all []*exercise.Exercise) *ExerciseRepository {
	copied := make([]*exercise.Exercise, len(all))
	copy(copied, all)
	return &ExerciseRepository{all: copied}
}

func (r *ExerciseRepository) FindAll(context.Context) ([]*exercise.Exercise, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	out := make([]*exercise.Exercise, len(r.all))
	copy(out, r.all)
	return out, nil
}

// SetLogRepository は実績ログを ID キーで保持する。
// 同じ ID を二度受けても重複しないため、Save は冪等になる。
type SetLogRepository struct {
	mu   sync.RWMutex
	byID map[setlog.SetLogID]*setlog.SetLog
}

func NewSetLogRepository() *SetLogRepository {
	return &SetLogRepository{byID: map[setlog.SetLogID]*setlog.SetLog{}}
}

func (r *SetLogRepository) FindAll(context.Context) (setlog.History, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// 取得順を安定させる。map の反復順は保証されないので、揺れると
	// History の重複解決や推定1RMの畳み込みが呼び出しごとに変わり、
	// 同じ入力から違う計画が出る。
	ids := make([]setlog.SetLogID, 0, len(r.byID))
	for id := range r.byID {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })

	out := make([]*setlog.SetLog, 0, len(ids))
	for _, id := range ids {
		out = append(out, r.byID[id])
	}
	return setlog.NewHistory(out), nil
}

// Save は冪等。同じ ID で同じ内容なら黙って受け入れ、内容が違えば
// ErrConflictingSetLog を返す。黙って上書きすると、リポジトリが返す順序
// 次第で推定1RMが変わり、同じ入力から違う計画が出る。
//
// 全か無かで書く。途中で衝突を見つけたら1件も書かない。半分だけ保存された
// 状態は、その週の刺激量を実態とずらしたまま計画に効き続ける。
func (r *SetLogRepository) Save(_ context.Context, logs []*setlog.SetLog) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	staged := make(map[setlog.SetLogID]*setlog.SetLog, len(logs))
	for i, l := range logs {
		if l == nil {
			return fmt.Errorf("%d番目のセットログが nil である", i)
		}
		for _, existing := range []*setlog.SetLog{r.byID[l.ID()], staged[l.ID()]} {
			if existing != nil && !existing.Equals(l) {
				return fmt.Errorf("%w: %s", setlog.ErrConflictingSetLog, l.ID())
			}
		}
		staged[l.ID()] = l
	}

	for id, l := range staged {
		r.byID[id] = l
	}
	return nil
}

// Delete は打ち間違いの訂正。存在しないIDでも成功として扱う。
func (r *SetLogRepository) Delete(_ context.Context, id setlog.SetLogID) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.byID, id)
	return nil
}

func (r *SetLogRepository) Size() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.byID)
}

// ConditionRepository は日次コンディションを日付キーで保持する。
// 同じ日付を二度受けたら上書きになるため、Save は冪等になる。
type ConditionRepository struct {
	mu     sync.RWMutex
	byDate map[string]condition.DailyCondition
}

func NewConditionRepository() *ConditionRepository {
	return &ConditionRepository{byDate: map[string]condition.DailyCondition{}}
}

func (r *ConditionRepository) FindAll(context.Context) (condition.ConditionLog, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// 並べ替えはしない。NewConditionLog が日付で整列し、同じ日付は
	// 項目ごとに合成するので、渡す順序は結果に影響しない。
	// ここで整列すると、意味のある処理に見えて実は何もしていない
	// コードが残る（実際、消しても全テストが通る状態だった）。
	out := make([]condition.DailyCondition, 0, len(r.byDate))
	for _, c := range r.byDate {
		out = append(out, c)
	}
	return condition.NewConditionLog(out), nil
}

// Save は冪等。同じ日付を二度受けたら項目ごとに上書きする。
//
// 日付ごと置き換えないのは、体重だけを送ったときに睡眠時間が消えるため。
// クライアントは体重と睡眠を別のタイミングで記録するので、これは日常的に起きる。
func (r *ConditionRepository) Save(_ context.Context, items []condition.DailyCondition) error {
	r.mu.Lock()
	defer r.mu.Unlock()

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
			base, ok = r.byDate[key]
		}
		if ok {
			c = base.Merge(c)
		}
		staged[key] = c
	}

	for k, c := range staged {
		r.byDate[k] = c
	}
	return nil
}

func (r *ConditionRepository) Size() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.byDate)
}

// ProgramRepository はユーザー設定を1つだけ保持する（単一ユーザー前提）。
type ProgramRepository struct {
	mu      sync.RWMutex
	program *program.Program
}

func NewProgramRepository(p *program.Program) *ProgramRepository {
	return &ProgramRepository{program: p}
}

func (r *ProgramRepository) Get(context.Context) (*program.Program, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if r.program == nil {
		return nil, program.ErrProgramNotConfigured
	}
	return r.program, nil
}

// Save は冪等。プログラムはユーザーごとに1つで、保存は常に全体の置き換え。
func (r *ProgramRepository) Save(_ context.Context, p *program.Program) error {
	if p == nil {
		return fmt.Errorf("プログラムが nil である")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.program = p
	return nil
}
