package exercise

import (
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/dyoshyy/liftplan/internal/domain/training"
)

// CustomExerciseIDPrefix は利用者が足した種目の ID の接頭辞。
//
// シードの ID は英小文字と "_" だけなので、この接頭辞とは衝突しない
// （seed の TestExercises_NoIDUsesTheCustomPrefix が守る）。
const CustomExerciseIDPrefix = "u-"

// 寄与の固定値。本人には「主に効く」「少し効く」しか選ばせない。
//
// 数値を入れさせないのは、1.0と0.5の違いを本人は答えられないから
// （CLAUDE.md「決めることを増やさない」）。0.5 はシードの副次寄与
// （0.3〜0.6）の真ん中に置いた。
const (
	primaryContribution   = 1.0
	secondaryContribution = 0.5
)

// maxCustomNameRunes は自分の種目の名前の上限。exercise.go の
// maxNameRunes（全種目に課す上限）に寄せてある。
const maxCustomNameRunes = maxNameRunes

// CustomExerciseParams は利用者が足す種目の生成入力。
type CustomExerciseParams struct {
	ID          string
	Name        string
	Primary     []training.MuscleRegion
	Secondary   []training.MuscleRegion
	IncrementKg float64
}

// NewCustomExercise は利用者が足す種目を組み立てる。
//
// 主に効く区分を1つ以上要求する。寄与1.0の区分が無い種目は、分割の
// どの日にも入らない（planning の isPrimaryIn）。
func NewCustomExercise(p CustomExerciseParams) (*Exercise, error) {
	if !strings.HasPrefix(p.ID, CustomExerciseIDPrefix) {
		return nil, fmt.Errorf("自分の種目の ID は %q で始まる必要がある: %q", CustomExerciseIDPrefix, p.ID)
	}
	if n := utf8.RuneCountInString(strings.TrimSpace(p.Name)); n > maxCustomNameRunes {
		return nil, fmt.Errorf("種目の名前が長すぎる: %d文字（上限 %d）", n, maxCustomNameRunes)
	}
	if len(p.Primary) == 0 {
		return nil, errors.New("主に効く部位を1つ以上選ぶ必要がある")
	}

	stimulus := make(map[training.MuscleRegion]float64, len(p.Primary)+len(p.Secondary))
	put := func(rs []training.MuscleRegion, v float64) error {
		for _, r := range rs {
			if _, dup := stimulus[r]; dup {
				return fmt.Errorf("部位 %s が2回選ばれている", r)
			}
			stimulus[r] = v
		}
		return nil
	}
	if err := put(p.Primary, primaryContribution); err != nil {
		return nil, err
	}
	if err := put(p.Secondary, secondaryContribution); err != nil {
		return nil, err
	}

	e, err := NewExercise(ExerciseParams{
		ID: p.ID, Name: p.Name, Stimulus: stimulus, IncrementKg: p.IncrementKg,
	})
	if err != nil {
		return nil, err
	}
	e.custom = true
	return e, nil
}

// NewRandomCustomExerciseID は自分の種目の ID を採番する。
//
// NewRandomExerciseID の旧名。呼び先を変えるだけの互換名として残す
// （Task 5 で削る）。
func NewRandomCustomExerciseID() (ExerciseID, error) {
	return NewRandomExerciseID()
}
