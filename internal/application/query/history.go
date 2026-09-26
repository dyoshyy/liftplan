// Package query は読み取り専用の経路。
//
// ユースケース（`usecase`）と分けているのは、responsibility が違うため。
// ユースケースはデータを集めてドメインに渡し、状態を進める。ここは
// 状態を変えず、既にある記録を読める形に整えるだけで、SessionPlanner を
// 通らない。
//
// 整形はここでやる。プレゼンテーション層に置くと、画面ごとに同じ集計を
// 書き直すことになる。ドメインに置くと、表示の都合がドメインに漏れる。
//
// **利用者は ctx の直後、第2引数で受け取る。**入力の構造体
// （GetSessionInput など）に混ぜない。入力は送り主が書いたリクエストから
// 組み立てられるので、所有者をそこに置くと、送り主が名乗った
// 名前で他人の記録を読み書きできる形が1回のミスで作れる。所有者は
// 認証から来るもので、入力から来るものではない。位置を全ての口で
// 揃えているのは、呼び出し側が並びを覚えずに済むようにするため。
//
// 種目マスタを読む Exercises だけは利用者を取らない。シード由来の静的な
// マスタで、誰が見ても同じものだから。
package query

import (
	"context"
	"fmt"
	"sort"

	"github.com/dyoshyy/liftplan/internal/application/apperror"
	"github.com/dyoshyy/liftplan/internal/domain/account"
	"github.com/dyoshyy/liftplan/internal/domain/training"
	"github.com/dyoshyy/liftplan/internal/domain/training/exercise"
	"github.com/dyoshyy/liftplan/internal/domain/training/setlog"
)

// Set は実績1セット。
type Set struct {
	ID       setlog.SetLogID
	WeightKg float64
	Reps     int
	RIR      int
}

// ExerciseLog は1つの種目で、その日にこなしたセットの集まり。
type ExerciseLog struct {
	ExerciseID exercise.ExerciseID
	Name       string
	Sets       []Set
}

// Day は1日ぶんの実績。
type Day struct {
	Date      training.Date
	Exercises []ExerciseLog
	TotalSets int
}

// LastPerformance はある種目の直近の実績。
//
// 今日提示された重量を信じる根拠になる。「92.5kg」と言われても、
// 前回90kgで潰れたのか余裕だったのかで、やることが変わる。
type LastPerformance struct {
	Date training.Date
	// WeightKg はその日の最も重いセット。次に何kgから入るかの目安になる。
	WeightKg float64
	// Weights と Reps はセットごとの実績で、順番も長さも揃っている。
	// 代表の1つに畳むと、ドロップセットも重量を上げた分も見えなくなる。
	Weights []float64
	Reps    []int
	DaysAgo int
}

// History は実績を読むための経路。
type History struct {
	logs      setlog.Reader
	exercises exercise.Reader
}

func NewHistory(
	logs setlog.Reader,
	exercises exercise.Reader,
) *History {
	return &History{logs: logs, exercises: exercises}
}

// Days は期間内の実績を、新しい日から順に返す。
func (q *History) Days(ctx context.Context, user account.UserID, from, to training.Date) (_ []Day, err error) {
	// 出口で1度だけ翻訳する。usecase と同じ形。ここを通らない公開メソッドは、
	// 一時障害を 500 で返す（#129）。
	defer func() { err = apperror.Classify(err) }()
	if from.IsZero() || to.IsZero() {
		return nil, fmt.Errorf("期間が指定されていない")
	}
	if to.Before(from) {
		return nil, fmt.Errorf("終わりが始まりより前である")
	}

	h, names, err := q.load(ctx, user)
	if err != nil {
		return nil, err
	}

	// TrainingSession は「同じ日」でまとめたもので、1日の中に複数の種目が
	// 入る。種目ごとに分けるのはここの仕事。
	byDate := map[training.Date]*Day{}
	order := []training.Date{}
	index := map[training.Date]map[exercise.ExerciseID]int{}

	for _, s := range h.OnOrAfter(from).OnOrBefore(to).Sessions() {
		day := &Day{Date: s.Date()}
		byDate[s.Date()] = day
		order = append(order, s.Date())
		index[s.Date()] = map[exercise.ExerciseID]int{}

		for _, l := range s.Logs() {
			id := l.ExerciseID()
			i, seen := index[s.Date()][id]
			if !seen {
				day.Exercises = append(day.Exercises, ExerciseLog{
					ExerciseID: id, Name: names[id],
				})
				i = len(day.Exercises) - 1
				index[s.Date()][id] = i
			}
			day.Exercises[i].Sets = append(day.Exercises[i].Sets, Set{
				ID:       l.ID(),
				WeightKg: l.Weight().Kg(),
				Reps:     l.Reps().Int(),
				RIR:      l.RIR().Int(),
			})
			day.TotalSets++
		}
	}

	// 新しい日から。振り返るときは直近から見る。
	sort.Slice(order, func(i, j int) bool { return order[j].Before(order[i]) })

	out := make([]Day, 0, len(order))
	for _, d := range order {
		if len(byDate[d].Exercises) > 0 {
			out = append(out, *byDate[d])
		}
	}
	return out, nil
}

// LastPerformances は種目ごとの直近の実績を返す。
//
// asOf より前の記録だけを見る。当日ぶんを含めると、いま記録した1セットが
// 「前回」として出てしまい、比較の意味が消える。
func (q *History) LastPerformances(
	ctx context.Context,
	user account.UserID,
	asOf training.Date,
) (_ map[exercise.ExerciseID]LastPerformance, err error) {
	defer func() { err = apperror.Classify(err) }()
	if asOf.IsZero() {
		return nil, fmt.Errorf("基準日が指定されていない")
	}

	h, _, err := q.load(ctx, user)
	if err != nil {
		return nil, err
	}

	// 種目ごとに、最も新しい日を選ぶ。1日に複数種目が入るので、
	// セッション単位ではなく種目単位で見る必要がある。
	latest := map[exercise.ExerciseID]training.Date{}
	logs := map[exercise.ExerciseID][]*setlog.SetLog{}

	for _, l := range h.Before(asOf).Logs() {
		id := l.ExerciseID()
		d := l.PerformedOn()
		if prev, ok := latest[id]; !ok || prev.Before(d) {
			latest[id] = d
			logs[id] = nil
		}
		if latest[id].Equal(d) {
			logs[id] = append(logs[id], l)
		}
	}

	out := make(map[exercise.ExerciseID]LastPerformance, len(latest))
	for id, date := range latest {
		ls := logs[id]
		if len(ls) == 0 {
			continue
		}
		reps := make([]int, 0, len(ls))
		weights := make([]float64, 0, len(ls))
		top := 0.0
		for _, l := range ls {
			reps = append(reps, l.Reps().Int())
			kg := l.Weight().Kg()
			weights = append(weights, kg)
			if kg > top {
				top = kg
			}
		}
		out[id] = LastPerformance{
			Date:     date,
			WeightKg: top,
			Weights:  weights,
			Reps:     reps,
			DaysAgo:  asOf.DaysSince(date),
		}
	}
	return out, nil
}

// load は履歴と種目名をまとめて取る。
func (q *History) load(ctx context.Context, user account.UserID) (
	setlog.History, map[exercise.ExerciseID]string, error,
) {
	if err := ctx.Err(); err != nil {
		return setlog.History{}, nil, fmt.Errorf("読み取りが中断された: %w", err)
	}

	h, err := q.logs.FindAll(ctx, user)
	if err != nil {
		return setlog.History{}, nil, fmt.Errorf("実績の取得に失敗: %w", err)
	}
	pool, err := q.exercises.FindAll(ctx, user)
	if err != nil {
		return setlog.History{}, nil, fmt.Errorf("種目の取得に失敗: %w", err)
	}

	names := make(map[exercise.ExerciseID]string, len(pool))
	for _, e := range pool {
		if e == nil {
			continue
		}
		names[e.ID()] = e.Name()
	}
	return h, names, nil
}
