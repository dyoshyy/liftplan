package usecase_test

import (
	"context"
	"errors"
	"testing"

	"github.com/dyoshyy/liftplan/internal/application/apperror"
	"github.com/dyoshyy/liftplan/internal/application/usecase"
	"github.com/dyoshyy/liftplan/internal/domain/training"
	"github.com/dyoshyy/liftplan/internal/domain/training/exercise"
	"github.com/dyoshyy/liftplan/internal/domain/training/program"
)

// 保存のときの「出られる日があるか」は、計画のときの「今日の軸の候補か」と
// 同じ境界で決まる。
//
// 境界がずれると、保存は通るのに二度と軸に出ない宣言種目ができる。他の
// 宣言が毎日1つは該当するのでフォールバックも発火せず、エラーも立たない
// まま消える。
//
// 種目を自前で組むのは、シードに寄与 0.9 台の区分が無いため（1.0 の次は
// デッドリフトの臀筋 0.8）。シードだけで書くと、境界を 0.9 に下げる変異が
// 緑のまま通る。
func TestSetSplitCycle_PrimaryBoundary(t *testing.T) {
	// ハムストリングが主働（1.0）、臀筋は主働に届かない（0.9）。
	nearPrimary, err := exercise.NewExercise(exercise.ExerciseParams{
		ID: "near_primary", Name: "near_primary",
		Stimulus: map[training.MuscleRegion]float64{
			training.Hamstring: 1.0, training.Glute: 0.9,
		},
		IncrementKg: 2.5,
	})
	if err != nil {
		t.Fatalf("種目の生成に失敗: %v", err)
	}

	cases := []struct {
		name    string
		regions []training.MuscleRegion
		wantErr error
	}{
		{
			// 臀筋 0.9 < 1.0。主働ではないので、計画はこの種目を
			// 臀筋の日の軸の候補にしない。保存も弾くこと。
			name:    "寄与が 1.0 未満の区分しか分割に入っていない宣言種目は弾かれる",
			regions: []training.MuscleRegion{training.Glute},
			wantErr: apperror.ErrInvalidInput,
		},
		{
			// ハムストリング 1.0 >= 1.0。
			name:    "寄与が 1.0 の区分が分割に入っていれば通る",
			regions: []training.MuscleRegion{training.Glute, training.Hamstring},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			freq, err := program.NewFrequency(3)
			if err != nil {
				t.Fatalf("頻度が不正: %v", err)
			}
			target, err := program.NewWeeklyVolumeTarget(
				map[training.MuscleRegion]float64{training.Hamstring: 10})
			if err != nil {
				t.Fatalf("週目標が不正: %v", err)
			}
			ids := []exercise.ExerciseID{"near_primary"}
			prog, err := program.NewProgram(freq, mustVolume(t, 6, 3), target, ids, ids, "")
			if err != nil {
				t.Fatalf("プログラムの生成に失敗: %v", err)
			}
			split, err := program.NewSplit("脚", c.regions)
			if err != nil {
				t.Fatalf("分割の生成に失敗: %v", err)
			}

			programs := &fakeProgram{program: prog}
			u := usecase.NewSetSplitCycle(
				&fakeExercises{all: []*exercise.Exercise{nearPrimary}}, programs, programs)
			err = u.Execute(context.Background(), testUser, []program.Split{split})

			if c.wantErr == nil {
				if err != nil {
					t.Fatalf("主働を含む周期が通らない: %v", err)
				}
				if programs.savedProgram() == nil {
					t.Error("保存されていない")
				}
				return
			}
			if !errors.Is(err, c.wantErr) {
				t.Errorf("エラーが %v。%v のはず", err, c.wantErr)
			}
			if programs.savedProgram() != nil {
				t.Error("弾いたのに保存された")
			}
		})
	}
}
