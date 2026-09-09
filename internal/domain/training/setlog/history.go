package setlog

import (
	"sort"

	"github.com/dyoshyy/liftplan/internal/domain/training"
	"github.com/dyoshyy/liftplan/internal/domain/training/exercise"
)

// TrainingSession は同一日に実施されたセットのまとまり。
type TrainingSession struct {
	date training.Date
	logs []*SetLog
}

func (s TrainingSession) Date() training.Date { return s.date }

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
func (s TrainingSession) MedianOneRepMax() (training.OneRepMax, bool) {
	values := make([]float64, 0, len(s.logs))
	for _, l := range s.logs {
		if v, ok := l.EstimatedOneRepMax(); ok {
			values = append(values, v.Kg())
		}
	}
	if len(values) == 0 {
		return training.OneRepMax{}, false
	}

	// 生成は必ずコンストラクタを通す。構造体リテラルで組み立てると、
	// コンストラクタが拒否する値が ok=true で流通してしまう。
	orm, err := training.NewOneRepMax(training.Median(values))
	if err != nil {
		return training.OneRepMax{}, false
	}
	return orm, true
}

// History は確定した実績の集まり。読み取り専用に扱う。
//
// 未来のセッションは保存せず、ここから毎回導出する。
// だから予定と実績が食い違う状態が原理的に発生しない。
type History struct {
	logs []*SetLog

	// lastPerformed は種目ごとの最終実施日の索引。
	// LastPerformed が何度も呼ばれるため、生成時に一度だけ構築する。
	lastPerformed map[exercise.ExerciseID]training.Date
}

// NewHistory は nil を除いたログの集まりを作る。
//
// 同じ ID のログが複数含まれる場合は、渡されたスライスの中で後にあるものを採用する。
// これは呼び出し側が渡した順序に依存する規則であって、リポジトリへの到着順とは
// 一致しない。リポジトリは ID をキーにしたマップで潰すため、そもそも重複が
// ここへ届かない。この規則が効くのは、既存の履歴に仮のログを重ねて
// 「もしこう記録したら」を試す場合だけ。
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
	return newHistory(out)
}

// newHistory は検証済みのログから History を組み立てる。
//
// 最終実施日を先に索引化しておく。LastPerformed は1回のセッション生成で
// 200回以上呼ばれるため、毎回全ログを走査すると履歴が伸びたときに効いてくる。
func newHistory(logs []*SetLog) History {
	index := make(map[exercise.ExerciseID]training.Date, len(logs))
	for _, l := range logs {
		if last, ok := index[l.ExerciseID()]; !ok || last.Before(l.PerformedOn()) {
			index[l.ExerciseID()] = l.PerformedOn()
		}
	}
	return History{logs: logs, lastPerformed: index}
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
	return newHistory(out)
}

func (h History) ForExercise(id exercise.ExerciseID) History {
	return h.filter(func(l *SetLog) bool { return l.ExerciseID() == id })
}

// OnOrAfter は d を含むそれ以降。上限を持たないので、対象日そのもののログも含む。
//
// 回復期間の判定に使うときは必ず `.Before(date)` で閉じること。
// 閉じないと、セッション中に記録してから計画を開き直したとき、
// たった今やった種目の筋区分が「最近刺激した」と判定され、
// そのセッションの補助枠から自分自身が消える。
func (h History) OnOrAfter(d training.Date) History {
	return h.filter(func(l *SetLog) bool { return !l.PerformedOn().Before(d) })
}

// Before は d を含まないそれ以前。
func (h History) Before(d training.Date) History {
	return h.filter(func(l *SetLog) bool { return l.PerformedOn().Before(d) })
}

// After は d を含まないそれ以降。
func (h History) After(d training.Date) History {
	return h.filter(func(l *SetLog) bool { return l.PerformedOn().After(d) })
}

// OnOrBefore は d を含むそれ以前。
//
// ある時点での推定を「その時点までに分かっていた情報だけ」で行うために使う。
// 絞り込まずに過去の推定をやり直すと、未来の記録が混ざる。
func (h History) OnOrBefore(d training.Date) History {
	return h.filter(func(l *SetLog) bool { return !l.PerformedOn().After(d) })
}

// Sessions は同一日ごとにまとめ、日付昇順で返す。
func (h History) Sessions() []TrainingSession {
	grouped := map[training.Date][]*SetLog{}
	for _, l := range h.logs {
		grouped[l.PerformedOn()] = append(grouped[l.PerformedOn()], l)
	}

	dates := make([]training.Date, 0, len(grouped))
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
	seen := map[training.Date]bool{}
	for _, l := range h.logs {
		seen[l.PerformedOn()] = true
	}
	return len(seen)
}

// LastPerformed はその種目を最後に実施した日。履歴が無ければ false。
func (h History) LastPerformed(id exercise.ExerciseID) (training.Date, bool) {
	d, ok := h.lastPerformed[id]
	return d, ok
}
