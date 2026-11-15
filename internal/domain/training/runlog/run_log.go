package runlog

import (
	"errors"
	"fmt"
	"math"
	"strings"

	"github.com/dyoshyy/liftplan/internal/domain/training"
)

const (
	// maxRunLogIDLen は識別子の長さの上限。セットログと同じ（ULID は26文字）。
	maxRunLogIDLen = 64

	// maxDistanceMeters は1回のランとして受け付ける距離の上限（200km）。
	//
	// 走れる距離の上限ではなく、打ち間違いの検出のため。メートルをキロの欄に
	// 入れる（5000m → 5000km）と、ここで止まる。
	maxDistanceMeters = 200_000

	// maxDurationSeconds は1回のランとして受け付ける時間の上限（24時間）。
	maxDurationSeconds = 24 * 60 * 60
)

// RunLogID はラン1回の同一性。
//
// セットログと同じく、クライアントが採番した ULID をそのまま使う。
// サーバーが振り直さないのは、同じ記録を二度送っても壊れない冪等性を保つため。
type RunLogID string

// NewRunLogID は識別子を検証する。規則はセットログの ID と同じ。
func NewRunLogID(s string) (RunLogID, error) {
	if s == "" {
		return "", errors.New("ランの記録のIDが空である")
	}
	if strings.TrimSpace(s) != s {
		return "", fmt.Errorf("ランの記録のIDの前後に空白がある: %q", s)
	}
	if len(s) > maxRunLogIDLen {
		return "", fmt.Errorf("ランの記録のIDが長すぎる: %dバイト", len(s))
	}
	for i, r := range s {
		if r < 0x20 || r == 0x7f {
			return "", fmt.Errorf("ランの記録のIDに制御文字が含まれる: 位置 %d", i)
		}
	}
	return RunLogID(s), nil
}

// Distance は走った距離。メートル単位の整数で持つ。
//
// 浮動小数で持たないのは、同じIDの再送を Equals で比べるため。km を
// そのまま持つと、5.2 と 5.2000000001 が別物になって衝突扱いになる。
type Distance struct{ m int }

// NewDistanceKm は km で受けた距離をメートルに丸めてから検証する。
func NewDistanceKm(km float64) (Distance, error) {
	if math.IsNaN(km) || math.IsInf(km, 0) {
		return Distance{}, fmt.Errorf("距離が数値ではない: %v", km)
	}
	m := math.Round(km * 1000)
	if m < 1 || m > maxDistanceMeters {
		return Distance{}, fmt.Errorf("距離は1m〜%dkmである必要がある: %vkm", maxDistanceMeters/1000, km)
	}
	return Distance{m: int(m)}, nil
}

// Meters はメートル単位の距離。
func (d Distance) Meters() int { return d.m }

// Km はキロメートル単位の距離。
func (d Distance) Km() float64 { return float64(d.m) / 1000 }

// Duration は走った時間。秒単位の整数で持つ。
type Duration struct{ s int }

// NewDurationSeconds は秒で受けた時間を検証する。
func NewDurationSeconds(s int) (Duration, error) {
	if s < 1 || s > maxDurationSeconds {
		return Duration{}, fmt.Errorf("時間は1秒〜24時間である必要がある: %d秒", s)
	}
	return Duration{s: s}, nil
}

// Seconds は秒単位の時間。
func (d Duration) Seconds() int { return d.s }

// RunLogParams は RunLog の生成入力。
type RunLogParams struct {
	ID              string
	PerformedOn     training.Date
	DistanceKm      float64
	DurationSeconds int
}

// RunLog は確定したラン1回。生成後は変更しない。
//
//ddd:aggregate
type RunLog struct {
	id          RunLogID
	performedOn training.Date
	distance    Distance
	duration    Duration
}

// NewRunLog は入力を検証して RunLog を組み立てる。
func NewRunLog(p RunLogParams) (*RunLog, error) {
	id, err := NewRunLogID(p.ID)
	if err != nil {
		return nil, err
	}
	if p.PerformedOn.IsZero() {
		return nil, fmt.Errorf("ランの記録 %s: 実施日が無い", id)
	}
	distance, err := NewDistanceKm(p.DistanceKm)
	if err != nil {
		return nil, fmt.Errorf("ランの記録 %s: %w", id, err)
	}
	duration, err := NewDurationSeconds(p.DurationSeconds)
	if err != nil {
		return nil, fmt.Errorf("ランの記録 %s: %w", id, err)
	}
	return &RunLog{id: id, performedOn: p.PerformedOn, distance: distance, duration: duration}, nil
}

func (l *RunLog) ID() RunLogID               { return l.id }
func (l *RunLog) PerformedOn() training.Date { return l.performedOn }
func (l *RunLog) Distance() Distance         { return l.distance }
func (l *RunLog) Duration() Duration         { return l.duration }

// Equals は ID を含む全ての値が一致するか。同じIDで内容の違う再送を
// 見分けるために使う（ErrConflictingRunLog）。
func (l *RunLog) Equals(o *RunLog) bool {
	if l == nil || o == nil {
		return l == o
	}
	return *l == *o
}
