package seed

import (
	"fmt"

	"github.com/dyoshyy/liftplan/internal/domain/training/exercise"
	"github.com/dyoshyy/liftplan/internal/domain/training/program"
)

// DefaultFrequencyPerWeek は初期プログラムの週あたりの頻度。
const DefaultFrequencyPerWeek = 3

// DefaultProgram はシードから初期プログラムを組む。
//
// バリエーションはメインに付随して自動で回るため、選択には含めない。
// 初期値を入れておくのは、設定を1つも持たない利用者が PUT /api/program を
// 叩かないと何も使えない状態を避けるため。設定はいつでも上書きできる。
//
// cmd/api/main.go の defaultProgram をここへ移した。移す前は「起動時に1人分」
// の話だったが、初回ログインでその利用者の分を作るようになり、呼ぶ側が
// 配線層とユースケースの2つになる。初期値は DefaultWeeklyTarget や
// DefaultDeclared と同じ「シードが決める値」であって、アプリケーション層の
// 手順ではない。ユースケース側に置くと、配線層が初期値を得るためだけに
// ユースケースを import することになる。
func DefaultProgram(pool []*exercise.Exercise) (*program.Program, error) {
	freq, err := program.NewFrequency(DefaultFrequencyPerWeek)
	if err != nil {
		return nil, fmt.Errorf("既定の頻度が不正: %w", err)
	}

	target, err := DefaultWeeklyTarget(freq)
	if err != nil {
		return nil, fmt.Errorf("週目標シードが不正: %w", err)
	}

	// 全種目を選んでおく。外したいものはあとから設定で外せる。
	selected := make([]exercise.ExerciseID, 0, len(pool))
	for _, e := range pool {
		selected = append(selected, e.ID())
	}
	// 重点種目は既定では指定しない。バリエーションレーンは空のまま回る。
	return program.NewProgram(freq, target, selected, DefaultDeclared(), "")
}
