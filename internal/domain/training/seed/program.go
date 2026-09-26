package seed

import (
	"fmt"

	"github.com/dyoshyy/liftplan/internal/domain/training/exercise"
	"github.com/dyoshyy/liftplan/internal/domain/training/program"
)

// DefaultFrequencyPerWeek は初期プログラムの週あたりの頻度。
const DefaultFrequencyPerWeek = 3

// 初期プログラムの1回の量。
//
// 4種目は「ジムで一度に集中が持つ上限」として置いた。以前は上限そのものが
// 無く、軸1＋補助8の9種目27セットが全頻度で固定的に出ていた。上から消して
// 好きなところでやめてよいキューのつもりでも、9種目並んでいれば全部やれという
// 圧になる。
//
// 3セットは変更前の軸・バリエーション・補助すべてと同じ値。ここを動かすと
// 変更の理由が2つ混ざるので、量を絞る判断だけを先に出す。
const (
	DefaultExercisesPerSession = 4
	DefaultSetsPerExercise     = 3
)

// DefaultSessionVolume は初期プログラムの1回の量。
func DefaultSessionVolume() (program.SessionVolume, error) {
	return program.NewSessionVolume(DefaultExercisesPerSession, DefaultSetsPerExercise)
}

// DefaultProgram はシードから初期プログラムを組む。
//
// バリエーションはメインに付随して自動で回るため、選択には含めない。
// 初期値を入れておくのは、設定を1つも持たない利用者が、先に何かを設定
// しないと何も使えない状態を避けるため。設定はいつでも上書きできる。
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

	volume, err := DefaultSessionVolume()
	if err != nil {
		return nil, fmt.Errorf("既定の1回の量が不正: %w", err)
	}

	target, err := DefaultWeeklyTarget(freq, volume)
	if err != nil {
		return nil, fmt.Errorf("週目標シードが不正: %w", err)
	}

	// 全種目を選んでおく。外したいものはあとから設定で外せる。
	selected := make([]exercise.ExerciseID, 0, len(pool))
	for _, e := range pool {
		selected = append(selected, e.ID())
	}
	// 重点種目は既定では指定しない。バリエーションレーンは空のまま回る。
	return program.NewProgram(freq, volume, target, selected, DefaultDeclared(), "")
}
