package planning

import (
	"github.com/dyoshyy/liftplan/internal/domain/training/exercise"
)

// DeloadProposal はデロードの提案。適用はしない。
//
// Reason はユーザーへ表示する根拠。黙って重量を下げないための情報。
type DeloadProposal struct {
	reason           string
	intensityDropPct float64
	stalled          []exercise.ExerciseID
}

func (p DeloadProposal) Reason() string            { return p.reason }
func (p DeloadProposal) IntensityDropPct() float64 { return p.intensityDropPct }

// StalledExercises は停滞と判定された種目。
//
// 根拠の文字列からしか取れないと、呼び出し側が「どの種目を下げるか」を
// 選べず、伸びている種目まで一律に下げることになる。
func (p DeloadProposal) StalledExercises() []exercise.ExerciseID {
	out := make([]exercise.ExerciseID, len(p.stalled))
	copy(out, p.stalled)
	return out
}

func (p DeloadProposal) IsZero() bool { return len(p.stalled) == 0 && p.reason == "" }
