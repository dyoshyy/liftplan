package planning

import (
	"github.com/dyoshyy/liftplan/internal/domain/training"
	"github.com/dyoshyy/liftplan/internal/domain/training/exercise"
	"github.com/dyoshyy/liftplan/internal/domain/training/program"
	"github.com/dyoshyy/liftplan/internal/domain/training/setlog"
)

// axis は今日の軸を返す。重点種目が軸に来た日も、他の宣言と同じく重い番で出す。
//
// 重点種目の番だけ 重い番 → 軽い番 → 派生 と一巡させていたが、やめた。
// 派生はバリエーションレーンが中1日空けて別の日に出すので、軸の側で軽い日を
// 作る理由が無い。
//
// 派生を軸に出さないので、上半身の日の軸が毎回重点種目になる分割では
// 派生がどこにも出ない（バリエーションレーンは軸が系統に含まれる日は出ない。
// D-125）。分割と重点種目を両方使っている利用者は今0人なので、手当てはしない。
// 要るようになったら、そのときの構成に合わせて足す。
//
// 重点種目の系統は、中1日空けてから軸に立てる（focusRested）。下半身の
// 宣言も、前日にやっていれば今日は軸に立てない（lowerBodyRested）。
//
// 両方は守れない日がある。昨日ラーセンとスクワットをやった日に、宣言が
// ベンチ・スクワット・デッドリフトだけなら候補が残らない。そのときは
// 重点種目の系統を休ませるほうを取る。胸は軸とバリエーションで週に何度も
// 来るが、下半身は軸でしか来ない。
//
// history は前日まで（Plan が切る）。
func axis(
	history setlog.History, prog *program.Program, pool, declared []*exercise.Exercise,
	today program.Split, hasSplit bool, date training.Date,
) *exercise.Exercise {
	rested := focusRested(history, prog, pool, declared, date)
	if lift := heavyLift(history, lowerBodyRested(history, rested, declared, date), today, hasSplit); lift != nil {
		return lift
	}
	if lift := heavyLift(history, rested, today, hasSplit); lift != nil {
		return lift
	}
	// 休ませたい系統しか今日の候補に無ければ、それを出す。軸を空に
	// すると、その日の主役が消える。
	return heavyLift(history, declared, today, hasSplit)
}

// lowerBodyRested は下半身の宣言を直近 variationRecoveryDays 日にやっていれば、
// 候補から下半身の宣言を除いて返す。
//
// 除かないと、宣言の順番が毎回同じに回るので RDL の翌回は必ずスクワットになる
// （TestSessionPlanner_AxisAlternatesLowerBody）。
//
// 見るのは宣言だけ。補助のレッグプレスをやった翌日にもスクワットは止めない。
// 補助まで見ると、ProjectHorizon は軸とバリエーションしか予測に書かないので、
// 予測と実際の計画がずれる（focusRested と同じ理由）。補助の側は
// AccessoryAllocator が回復中の区分を避けている。
//
// 主働の重なりで判定しない。スクワットの主働は大腿四頭、デッドリフトは
// ハムストリングと脊柱起立筋で、重ならない。
//
// 重点種目が下半身のとき、前日のバリエーション（ポーズスクワットなど）は
// 見ていないので、翌日にデッドリフトが立つ。下半身を重点にしている利用者は
// 今0人なので、手当てはしない。足すなら重点種目の系統も見ればよい
// （バリエーションは予測に書かれるので、予測とずれない）。
func lowerBodyRested(
	history setlog.History, candidates, declared []*exercise.Exercise, date training.Date,
) []*exercise.Exercise {
	if !recentlyPerformed(history, lowerBodyOf(declared), date) {
		return candidates
	}
	out := make([]*exercise.Exercise, 0, len(candidates))
	for _, e := range candidates {
		if !isLowerBody(e) {
			out = append(out, e)
		}
	}
	return out
}

// lowerBodyOf は種目のうち下半身のものを返す。
func lowerBodyOf(exercises []*exercise.Exercise) []*exercise.Exercise {
	out := make([]*exercise.Exercise, 0, len(exercises))
	for _, e := range exercises {
		if isLowerBody(e) {
			out = append(out, e)
		}
	}
	return out
}

// isLowerBody は主働（寄与 primaryContribution 以上）のどれかが下半身の区分か。
func isLowerBody(e *exercise.Exercise) bool {
	for _, r := range e.Stimulus().Regions() {
		c, ok := e.Stimulus().Contribution(r)
		if ok && c.Float() >= primaryContribution && r.IsLowerBody() {
			return true
		}
	}
	return false
}

// focusRested は宣言のうち、重点種目の系統を直近 variationRecoveryDays 日に
// やっていれば、その系統に属するものを除いて返す。
//
// バリエーションレーンは系統を中1日空けて出す（variationLift）。軸の側が
// それを見ないと、月曜にバリエーションで派生、火曜に軸で本体が出る。
// 軸は「宣言のうち最も古いもの」なので、派生をやっても本体は古いまま
// 選ばれる。
//
// 重点種目の系統だけを見る。重点でない宣言の派生が補助で出た翌日に本体が
// 軸に立つことは止めない。それを止めるには補助の予測まで履歴に入れる
// 必要があるが、ProjectHorizon は軸とバリエーションしか予測に書かないので、
// 予測と実際の計画がずれる。
//
// 除く対象は宣言の中の系統なので、RDL を宣言して重点をデッドリフトに
// した場合は RDL も休ませる（lineage に含まれる）。
func focusRested(
	history setlog.History, prog *program.Program, pool, declared []*exercise.Exercise,
	date training.Date,
) []*exercise.Exercise {
	focus, ok := prog.FocusExercise()
	if !ok {
		return declared
	}
	family := lineage(pool, focus)
	if !recentlyPerformed(history, family, date) {
		return declared
	}
	out := make([]*exercise.Exercise, 0, len(declared))
	for _, e := range declared {
		if !containsExercise(family, e.ID()) {
			out = append(out, e)
		}
	}
	return out
}

// heavyLift は今日メインでやる＝高重量を扱う種目を返す。
//
// 宣言のうち、最後に実施したのが最も古い種目を返す。未着手の種目が
// あればそれを優先する。
//
// 分割があれば、その日の区分を主働に含む宣言だけが候補になる。該当が
// 無ければ nil。フォールバックで別の日の種目を出すと、その日だけ分割が
// 意味を失う。
func heavyLift(
	history setlog.History, declared []*exercise.Exercise,
	today program.Split, hasSplit bool,
) *exercise.Exercise {
	candidates := declared
	if hasSplit {
		candidates = primaryIn(candidates, today)
	}
	return stalest(history, candidates)
}

// stalest は候補のうち、最後に実施したのが最も古い種目を返す。候補が空なら nil。
//
// 未着手の種目があればそれを優先する。記録が無いのを「最も古い」と解釈する
// ため、ゼロ値の日付と比べるのではなく LastPerformed の第2返り値で分ける。
// 日付のゼロ値が何を表すかを知らなくても読める。
//
// 同点は先に見たものを残す。候補は usablePool が ID 昇順に並べているので、
// 同じ入力から同じ種目が返る。
//
// 渡す履歴は前日まで。理由は Plan に書いた。
func stalest(h setlog.History, candidates []*exercise.Exercise) *exercise.Exercise {
	var best *exercise.Exercise
	var bestDate training.Date

	for _, c := range candidates {
		last, ok := h.LastPerformed(c.ID())
		if !ok {
			return c
		}
		if best == nil || last.Before(bestDate) {
			best, bestDate = c, last
		}
	}
	return best
}
