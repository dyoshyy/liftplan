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
	"github.com/dyoshyy/liftplan/internal/domain/training/seed"
)

// 種目マスタとの突合（verifySelection）を、それを呼ぶ狭い口から見る。
//
// 元は全置換の ConfigureProgram のテストが見ていた。全置換の口を消した
// （#123）ので、同じ性質を SetSelectedExercises の側で固定する。
// SetWeeklyTarget の側でも見ていたが、週目標を手で変える口ごと消した（D-139）。HTTP 越しのテスト（handler_test.go）はステータスしか見ておらず、
// 「弾いたうえで保存していない」「nil 混じりのマスタで落ちない」
// 「マスタが読めないときに入力の不正と言わない」は観測できない。

// bicepsProgram は「上腕二頭筋だけを狙い、カールで埋める」プログラム。
//
// 週目標が入力できた頃は「週目標と噛み合わない選択」を作るために使って
// いたが、その検査は削った（#176、verifySelection のコメント参照）。
// 種目マスタとの突合だけを見る他のケースにも使い回せるよう、名前は残す。
func bicepsProgram(t *testing.T) *program.Program {
	t.Helper()
	freq, err := program.NewFrequency(3)
	if err != nil {
		t.Fatalf("頻度が不正: %v", err)
	}
	p, err := program.NewProgram(freq, mustVolume(t, 6, 3),
		[]exercise.ExerciseID{"squat", "calf_raise", "barbell_curl"},
		[]exercise.ExerciseID{"squat"}, "")
	if err != nil {
		t.Fatalf("プログラムが不正: %v", err)
	}
	return p
}

func TestSetSelectedExercises_VerifiesAgainstTheExerciseMaster(t *testing.T) {
	pool, err := seed.Exercises()
	if err != nil {
		t.Fatalf("シードが不正: %v", err)
	}
	boom := errors.New("読めない")

	cases := []struct {
		name      string
		exercises *fakeExercises
		ids       []exercise.ExerciseID
		// wantIs は返るエラーが辿れるべきもの。空なら成功するはず。
		wantIs []error
		// wantNot は辿れてはいけないもの。
		wantNot   error
		wantSaved bool
	}{
		{
			// 黙って受け入れると SessionPlanner が黙って落とし、選んだ種目が
			// 理由の説明なくメニューから消える。
			name:      "種目マスタに無い種目は入力の不正として弾き、保存しない",
			exercises: &fakeExercises{all: pool},
			ids:       []exercise.ExerciseID{"squat", "barbell_curl", "存在しない種目"},
			wantIs:    []error{apperror.ErrInvalidInput, exercise.ErrExerciseNotFound},
		},
		{
			// nil を読み飛ばさないと e.ID() で panic する。
			name:      "nil 混じりの種目マスタでも落ちずに保存する",
			exercises: &fakeExercises{all: append([]*exercise.Exercise{nil}, pool...)},
			ids:       []exercise.ExerciseID{"squat", "barbell_curl"},
			wantSaved: true,
		},
		{
			// 検証できないものは保存しない。入力の不正と言ってしまうと、
			// クライアントは送り直せば通るものを捨てる。
			name:      "種目マスタが読めないなら保存せず、入力の不正とも言わない",
			exercises: &fakeExercises{err: boom},
			ids:       []exercise.ExerciseID{"squat", "barbell_curl"},
			wantIs:    []error{boom},
			wantNot:   apperror.ErrInvalidInput,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			programs := &fakeProgram{program: bicepsProgram(t)}
			uc := usecase.NewSetSelectedExercises(c.exercises, programs, programs)

			err := uc.Execute(context.Background(), testUser, c.ids)

			if len(c.wantIs) == 0 && err != nil {
				t.Fatalf("実行に失敗: %v", err)
			}
			for _, want := range c.wantIs {
				if !errors.Is(err, want) {
					t.Errorf("エラーから %v が辿れない: %v", want, err)
				}
			}
			if c.wantNot != nil && errors.Is(err, c.wantNot) {
				t.Errorf("エラーから %v が辿れてしまう: %v", c.wantNot, err)
			}
			if saved := programs.savedProgram() != nil; saved != c.wantSaved {
				t.Errorf("保存されたか が %v。%v のはず", saved, c.wantSaved)
			}
		})
	}
}

// 消した種目を選択に戻せないこと。古い画面から PUT /api/program/selected
// されたときに、計画に消した種目が戻ってくる。
//
// verifySelection は非公開で、このファイルは外部テスト
// （usecase_test）なので直接は呼べない。同じ性質を、それを呼ぶ
// SetSelectedExercises.Execute から見る。
func TestSetSelectedExercises_RejectsDeletedExercise(t *testing.T) {
	pool, err := seed.Exercises()
	if err != nil {
		t.Fatalf("シードが不正: %v", err)
	}
	e, err := exercise.NewCustomExercise(exercise.CustomExerciseParams{
		ID:          "u-000000000000000a",
		Name:        "アイソラテラル・ロー",
		Primary:     []training.MuscleRegion{training.Lat},
		IncrementKg: 2.5,
	})
	if err != nil {
		t.Fatalf("種目が不正: %v", err)
	}
	pool = append(pool, e.Delete())

	programs := &fakeProgram{program: bicepsProgram(t)}
	uc := usecase.NewSetSelectedExercises(&fakeExercises{all: pool}, programs, programs)

	ids := []exercise.ExerciseID{"squat", "calf_raise", "barbell_curl", "u-000000000000000a"}
	err = uc.Execute(context.Background(), testUser, ids)

	if !errors.Is(err, apperror.ErrInvalidInput) {
		t.Errorf("消した種目が選べた: %v", err)
	}
	if programs.savedProgram() != nil {
		t.Error("拒否したはずなのに保存された")
	}
}
