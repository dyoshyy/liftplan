package usecase_test

import (
	"context"
	"errors"
	"testing"

	"github.com/dyoshyy/liftplan/internal/application/apperror"
	"github.com/dyoshyy/liftplan/internal/application/usecase"
	"github.com/dyoshyy/liftplan/internal/domain/training/exercise"
	"github.com/dyoshyy/liftplan/internal/domain/training/program"
	"github.com/dyoshyy/liftplan/internal/domain/training/seed"
)

// 5分割を選んでいる利用者が頻度を週4回未満へ下げようとすると弾かれる。
//
// 自動でプリセットを外す（フォールバック）のではなく拒否する。分割を残した
// まま頻度だけ下げると、その周期が要求する頻度を下回ったまま黙って進む。
// 「分割を変えるかどうか」は本人の判断で、アプリが代わりに外してはいけない
// （CLAUDE.md「答えるべき問いと、答えるべきでない問いを分ける」）。
//
// SetSplitCycle 側のテスト（プリセットを選ぶとき）と対で、頻度を下げる
// ときにも同じ下限が効くことを見る。
func TestSetFrequency_RejectsBelowSplitMinimum(t *testing.T) {
	presets, err := seed.SplitPresets()
	if err != nil {
		t.Fatalf("プリセットが不正: %v", err)
	}
	var fiveWay, ppl []program.Split
	for _, p := range presets {
		switch p.Key {
		case "five_way":
			fiveWay = p.Cycle
		case "ppl":
			ppl = p.Cycle
		}
	}
	if fiveWay == nil || ppl == nil {
		t.Fatal("必要なプリセットが見つからない")
	}

	ids := []exercise.ExerciseID{"bench"}

	cases := []struct {
		name    string
		cycle   []program.Split
		perWeek int
		wantErr bool
	}{
		{
			name:  "5分割のまま週3回に下げようとすると弾かれる",
			cycle: fiveWay, perWeek: 3, wantErr: true,
		},
		{
			name:  "5分割のまま週4回はそのまま通る",
			cycle: fiveWay, perWeek: 4, wantErr: false,
		},
		{
			name:  "PPLには下限が無いので週2回でも通る",
			cycle: ppl, perWeek: 2, wantErr: false,
		},
		{
			name:  "分割なしなら下限が無い",
			cycle: nil, perWeek: 1, wantErr: false,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			freq, err := program.NewFrequency(4)
			if err != nil {
				t.Fatalf("頻度が不正: %v", err)
			}
			base, err := program.NewProgram(freq, mustVolume(t, 6, 3), ids, ids, "")
			if err != nil {
				t.Fatalf("プログラムの生成に失敗: %v", err)
			}
			prog, err := base.WithCycle(c.cycle)
			if err != nil {
				t.Fatalf("分割の設定に失敗: %v", err)
			}

			programs := &fakeProgram{program: prog}
			u := usecase.NewSetFrequency(programs, programs)
			err = u.Execute(context.Background(), testUser, c.perWeek)

			if c.wantErr {
				if !errors.Is(err, apperror.ErrInvalidInput) {
					t.Fatalf("エラーが %v。%v のはず", err, apperror.ErrInvalidInput)
				}
				if programs.savedProgram() != nil {
					t.Error("弾いたのに保存された")
				}
				return
			}
			if err != nil {
				t.Fatalf("通るはずが失敗: %v", err)
			}
			if programs.savedProgram() == nil {
				t.Error("保存されていない")
			}
		})
	}
}
