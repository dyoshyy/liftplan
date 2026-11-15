package planning

import (
	"cmp"
	"slices"

	"github.com/dyoshyy/liftplan/internal/domain/training"
)

// cardioDaysPerWeek は週に置く有酸素の日数。
//
// 利用者の設定にしないのは、「何をやるか」はアプリが答える問いだから
// （CLAUDE.md「決めなくていいためのアプリで、決めることを増やさない」）。
// 休みが足りなければ、ある分だけ置く。
const cardioDaysPerWeek = 2

// CardioDay は有酸素を置く休みの日1つ。
//
// 日付ではなく「何回目の練習の後の、休みの何日目か」で表す。周期は出席
// 回数で進み、練習の日付は horizonDates の仮置きにすぎない。実際に通う
// 曜日がずれても、「脚の日の翌日かどうか」は練習の並びだけで決まるので
// 崩れない。
//
// 日付は持たない。予定の応答が日付を返さないのと同じ理由で、持てば
// いずれ画面に出る（docs/specs/2026-09-27-session-forecast-design.md）。
type CardioDay struct {
	afterSession int
	restDay      int
	afterLegDay  bool
}

// AfterSession は直前の練習の回。ProjectHorizon の戻り値の添字（0 が今日）。
func (c CardioDay) AfterSession() int { return c.afterSession }

// RestDay はその回の後の休みの何日目か。1 が練習の翌日。
func (c CardioDay) RestDay() int { return c.restDay }

// PlaceCardio は ProjectHorizon の1週間ぶんの予測から、有酸素を置く休みの日を
// 選び、早い順に返す。
//
// 練習の日には置かない。休みのうち、前日が脚の日でないものを先に、
// 前日が脚の日のものを後に、それぞれ早い日から cardioDaysPerWeek 日取る。
// 脚を鍛えた翌日に脚を回す有酸素を重ねないため。
//
// 最後の回の後の休みは、来週の今日（sessions[0] の7日後）の前日まで数える。
// 予測は毎回作り直すので、その先は次に導出したときの回0から始まる。
func PlaceCardio(sessions []ProjectedSession) []CardioDay {
	if len(sessions) == 0 {
		return nil
	}
	nextWeek := sessions[0].Date().AddDays(7)

	var rest []CardioDay
	for k, s := range sessions {
		next := nextWeek
		if k+1 < len(sessions) {
			next = sessions[k+1].Date()
		}
		leg := isLegDay(s)
		for n := 1; n < next.DaysSince(s.Date()); n++ {
			rest = append(rest, CardioDay{
				afterSession: k, restDay: n,
				afterLegDay: leg && n == 1,
			})
		}
	}

	// 安定ソートなので、同じ優先度の中では早い順が保たれる。
	slices.SortStableFunc(rest, func(a, b CardioDay) int {
		switch {
		case a.afterLegDay == b.afterLegDay:
			return 0
		case b.afterLegDay:
			return -1
		default:
			return 1
		}
	})
	picked := rest[:min(cardioDaysPerWeek, len(rest))]
	slices.SortFunc(picked, func(a, b CardioDay) int {
		return cmp.Or(cmp.Compare(a.afterSession, b.afterSession), cmp.Compare(a.restDay, b.restDay))
	})
	return picked
}

// isLegDay はその回が脚を鍛える日か。
//
// 分割があれば分割の区分で、分割が無ければ（全区分の日）軸で決める。
// 全区分の日を一律に脚の日にすると、分割を組まない人は練習の翌日が
// 全部後回しになる。補助が脚に入ることは見ない。補助は軸と
// バリエーションの後に割り振るので、ProjectHorizon の時点ではまだ決まっていない。
func isLegDay(s ProjectedSession) bool {
	if split, ok := s.Split(); ok {
		for _, r := range training.LowerBody() {
			if isLegRegion(r) && split.Includes(r) {
				return true
			}
		}
		return false
	}
	axis, _, ok := s.Axis()
	if !ok {
		return false
	}
	for _, r := range axis.Stimulus().Regions() {
		c, ok := axis.Stimulus().Contribution(r)
		if ok && c.Float() >= primaryContribution && isLegRegion(r) {
			return true
		}
	}
	return false
}

// isLegRegion は脚の区分か。下半身（LowerBody）から脊柱起立筋を除いたもの。
//
// LowerBody が脊柱起立筋を含むのは、デッドリフトとスクワットが両方とも
// 強く使うから。だがそのまま使うと、PPL の「引く」や5分割の「背中」まで
// 脚の日になり、翌日の休みが後回しになる。脚を回す有酸素とぶつかるのは
// 脚そのもので、脊柱起立筋ではない。
func isLegRegion(r training.MuscleRegion) bool {
	return r.IsLowerBody() && r != training.Erector
}
