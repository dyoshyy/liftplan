package training

import "sort"

// TrainingSession は同一日に実施されたセットのまとまり。
type TrainingSession struct {
	date Date
	logs []*SetLog
}

func (s TrainingSession) Date() Date { return s.date }

func (s TrainingSession) Logs() []*SetLog {
	out := make([]*SetLog, len(s.logs))
	copy(out, s.logs)
	return out
}

func (s TrainingSession) IsEmpty() bool { return len(s.logs) == 0 }

// MedianOneRepMax はこのセッションを代表する推定1RM。
//
// 中央値を使うのは、セッション内に1セットだけ異常な記録が混ざっても
// 引きずられないため。仕様の「明らかな外れ値は除外する」をこの形で満たす。
//
// 推定できないセット（自重種目、Epley 式の適用範囲外）は必ず除外する。
// 0 として混ぜると、加重ディップス2セット + 自重3セットという日常的な
// セッションで代表値が 46kg から 0kg に崩壊する。
// 推定できるセットが1つも無ければ false を返す。
func (s TrainingSession) MedianOneRepMax() (OneRepMax, bool) {
	values := make([]float64, 0, len(s.logs))
	for _, l := range s.logs {
		if v, ok := l.EstimatedOneRepMax(); ok {
			values = append(values, v.Kg())
		}
	}
	if len(values) == 0 {
		return OneRepMax{}, false
	}

	// 生成は必ずコンストラクタを通す。構造体リテラルで組み立てると、
	// コンストラクタが拒否する値が ok=true で流通してしまう。
	orm, err := NewOneRepMax(median(values))
	if err != nil {
		return OneRepMax{}, false
	}
	return orm, true
}

// median は昇順ソートした上での中央値。呼び出し側が非空を保証すること。
//
// 量子化はここでは行わない。生成を必ず NewOneRepMax に通すことで、
// 検証と量子化を一箇所に集約する。
func median(values []float64) float64 {
	sorted := make([]float64, len(values))
	copy(sorted, values)
	sort.Float64s(sorted)

	n := len(sorted)
	if n%2 == 1 {
		return sorted[n/2]
	}
	return (sorted[n/2-1] + sorted[n/2]) / 2
}

// History は確定した実績の集まり。読み取り専用に扱う。
//
// 未来のセッションは保存せず、ここから毎回導出する。
// だから予定と実績が食い違う状態が原理的に発生しない。
type History struct {
	logs []*SetLog
}

// NewHistory は nil を除いたログの集まりを作る。
//
// 同じ ID のログが複数含まれる場合は、後のものを採用する。
// リポジトリが冪等に上書きする挙動と揃えている。
func NewHistory(logs []*SetLog) History {
	seen := make(map[SetLogID]int, len(logs))
	out := make([]*SetLog, 0, len(logs))

	for _, l := range logs {
		if l == nil {
			continue
		}
		if i, dup := seen[l.ID()]; dup {
			out[i] = l
			continue
		}
		seen[l.ID()] = len(out)
		out = append(out, l)
	}
	return History{logs: out}
}

func (h History) IsEmpty() bool { return len(h.logs) == 0 }

func (h History) Len() int { return len(h.logs) }

func (h History) Logs() []*SetLog {
	out := make([]*SetLog, len(h.logs))
	copy(out, h.logs)
	return out
}

func (h History) filter(keep func(*SetLog) bool) History {
	out := make([]*SetLog, 0, len(h.logs))
	for _, l := range h.logs {
		if keep(l) {
			out = append(out, l)
		}
	}
	return History{logs: out}
}

func (h History) ForExercise(id ExerciseID) History {
	return h.filter(func(l *SetLog) bool { return l.ExerciseID() == id })
}

// OnOrAfter は d を含むそれ以降。
func (h History) OnOrAfter(d Date) History {
	return h.filter(func(l *SetLog) bool { return !l.PerformedOn().Before(d) })
}

// Before は d を含まないそれ以前。
func (h History) Before(d Date) History {
	return h.filter(func(l *SetLog) bool { return l.PerformedOn().Before(d) })
}

// Sessions は同一日ごとにまとめ、日付昇順で返す。
func (h History) Sessions() []TrainingSession {
	grouped := map[Date][]*SetLog{}
	for _, l := range h.logs {
		grouped[l.PerformedOn()] = append(grouped[l.PerformedOn()], l)
	}

	dates := make([]Date, 0, len(grouped))
	for d := range grouped {
		dates = append(dates, d)
	}
	sort.Slice(dates, func(i, j int) bool { return dates[i].Before(dates[j]) })

	out := make([]TrainingSession, 0, len(dates))
	for _, d := range dates {
		out = append(out, TrainingSession{date: d, logs: grouped[d]})
	}
	return out
}

func (h History) SessionCount() int {
	seen := map[Date]bool{}
	for _, l := range h.logs {
		seen[l.PerformedOn()] = true
	}
	return len(seen)
}

// LastPerformed はその種目を最後に実施した日。履歴が無ければ false。
func (h History) LastPerformed(id ExerciseID) (Date, bool) {
	var last Date
	found := false
	for _, l := range h.logs {
		if l.ExerciseID() != id {
			continue
		}
		if !found || last.Before(l.PerformedOn()) {
			last = l.PerformedOn()
			found = true
		}
	}
	return last, found
}
