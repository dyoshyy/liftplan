package training

import (
	"errors"
	"fmt"
	"strings"
)

// maxSetLogIDLen は識別子の長さの上限。ULID は26文字だが、
// 将来別の採番方式に変わる余地を残しつつ、青天井は避ける。
const maxSetLogIDLen = 64

// SetLogID は実績1セットの同一性。
//
// クライアントが採番した ULID をそのまま使う。サーバーが振り直さないのは、
// 同じログを二度送っても壊れない冪等性を保つため。
type SetLogID string

func NewSetLogID(s string) (SetLogID, error) {
	if s == "" {
		return "", errors.New("セットログIDが空である")
	}
	if strings.TrimSpace(s) != s {
		return "", fmt.Errorf("セットログIDの前後に空白がある: %q", s)
	}
	if strings.TrimSpace(s) == "" {
		return "", errors.New("セットログIDが空白のみである")
	}
	if len(s) > maxSetLogIDLen {
		return "", fmt.Errorf("セットログIDが長すぎる: %d文字", len(s))
	}
	return SetLogID(s), nil
}

// SetLogParams は SetLog の生成入力。
type SetLogParams struct {
	ID          string
	PerformedOn Date
	ExerciseID  string
	WeightKg    float64
	Reps        int
	RIR         int
}

// SetLog は確定した実績1セット。
//
// エンジンにとって唯一の真実であり、生成後は変更しない。
// 未来のセッションを保存せずここから毎回導出するため、
// 予定と実績が食い違う状態が原理的に発生しない。
type SetLog struct {
	id          SetLogID
	performedOn Date
	exerciseID  ExerciseID
	weight      Weight
	reps        Reps
	rir         RIR
}

func NewSetLog(p SetLogParams) (*SetLog, error) {
	id, err := NewSetLogID(p.ID)
	if err != nil {
		return nil, err
	}
	if p.PerformedOn.IsZero() {
		return nil, fmt.Errorf("セットログ %s: 実施日が無い", id)
	}
	exerciseID, err := NewExerciseID(p.ExerciseID)
	if err != nil {
		return nil, fmt.Errorf("セットログ %s: %w", id, err)
	}
	weight, err := NewWeight(p.WeightKg)
	if err != nil {
		return nil, fmt.Errorf("セットログ %s: %w", id, err)
	}
	reps, err := NewReps(p.Reps)
	if err != nil {
		return nil, fmt.Errorf("セットログ %s: %w", id, err)
	}
	rir, err := NewRIR(p.RIR)
	if err != nil {
		return nil, fmt.Errorf("セットログ %s: %w", id, err)
	}

	return &SetLog{
		id:          id,
		performedOn: p.PerformedOn,
		exerciseID:  exerciseID,
		weight:      weight,
		reps:        reps,
		rir:         rir,
	}, nil
}

func (s *SetLog) ID() SetLogID           { return s.id }
func (s *SetLog) PerformedOn() Date      { return s.performedOn }
func (s *SetLog) ExerciseID() ExerciseID { return s.exerciseID }
func (s *SetLog) Weight() Weight         { return s.weight }
func (s *SetLog) Reps() Reps             { return s.reps }
func (s *SetLog) RIR() RIR               { return s.rir }

// EstimatedOneRepMax はこの1セットから推定される1RM。
//
// 推定できない場合は false を返す。自重種目（0kg）と、
// Epley 式の適用範囲を超える高レップの記録がこれにあたる。
// 呼び出し側は false のセットを平均や中央値から除外すること。
// 0 として混ぜると、その日の代表値が実態より低くなる。
func (s *SetLog) EstimatedOneRepMax() (OneRepMax, bool) {
	return EstimateOneRepMax(s.weight, s.reps, s.rir)
}

// SameIdentity はエンティティの同一性判定。値ではなく ID で比べる。
func (s *SetLog) SameIdentity(o *SetLog) bool {
	if s == nil || o == nil {
		return false
	}
	return s.id == o.id
}
