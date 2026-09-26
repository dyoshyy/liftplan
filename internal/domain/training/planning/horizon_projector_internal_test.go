package planning

import (
	"testing"
	"time"

	"github.com/dyoshyy/liftplan/internal/domain/training"
	"github.com/dyoshyy/liftplan/internal/domain/training/exercise"
	"github.com/dyoshyy/liftplan/internal/domain/training/program"
	"github.com/dyoshyy/liftplan/internal/domain/training/setlog"
)

// axisLaneRole は非公開なので、パッケージ内から直接検査する。
//
// 呼び出し側（外部テスト）から見えるのは Axis() が返すセット数だけで、
// heavyRole（0.88・3レップ相当）と focusVolumeRole（0.81・6レップ相当）の
// どちらで出たかは出力に現れない。Forecast がこの役割を取り違えると、
// 先の回の重点種目の一巡（1回目は重い・2回目はボリューム）が全部
// heavyRole 扱いになり、一巡2番目の回の処方が実際より重く出る。
func TestProjectedSession_AxisLaneRoleFollowsTheFocusCycle(t *testing.T) {
	bench, err := exercise.NewExercise(exercise.ExerciseParams{
		ID: "bench", Name: "ベンチプレス",
		Stimulus:    map[training.MuscleRegion]float64{training.ChestMid: 1.0},
		IncrementKg: 2.5,
	})
	if err != nil {
		t.Fatalf("種目の生成に失敗: %v", err)
	}
	freq, err := program.NewFrequency(7)
	if err != nil {
		t.Fatalf("頻度が不正: %v", err)
	}
	volume, err := program.NewSessionVolume(6, 3)
	if err != nil {
		t.Fatalf("1回の量が不正: %v", err)
	}
	// 宣言も重点種目も bench 1つだけ。派生を選択していないので、一巡の
	// 3番目（派生の番）は該当が無く本体へフォールバックする
	// （axis_rotation.go の axis のコメントどおり）。
	prog, err := program.NewProgram(freq, volume,
		[]exercise.ExerciseID{"bench"}, []exercise.ExerciseID{"bench"}, "bench")
	if err != nil {
		t.Fatalf("プログラムの生成に失敗: %v", err)
	}
	date := training.MustDate(2026, time.August, 17)

	got, err := DefaultSessionPlanner().ProjectHorizon(
		setlog.NewHistory(nil), prog, []*exercise.Exercise{bench}, date)
	if err != nil {
		t.Fatalf("ProjectHorizon が失敗: %v", err)
	}
	if len(got) != 7 {
		t.Fatalf("回数が %d。頻度7のはず", len(got))
	}

	// 位置0→heavy, 1→focusVolume, 2→派生無しでheavyへフォールバック、を
	// 3回ぶん繰り返す（focusCycleLength=3、宣言1つだけなので毎回進む）。
	want := []laneRole{
		heavyRole, focusVolumeRole, heavyRole,
		heavyRole, focusVolumeRole, heavyRole, heavyRole,
	}
	for k, sess := range got {
		if role := sess.axisLaneRole(); role != want[k] {
			t.Errorf("回%d: 役割が %v。%v のはず", k, role, want[k])
		}
	}
}
