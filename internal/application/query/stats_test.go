package query_test

import (
	"context"
	"math"
	"testing"

	"github.com/dyoshyy/liftplan/internal/domain/account"

	"github.com/dyoshyy/liftplan/internal/application/query"
	"github.com/dyoshyy/liftplan/internal/domain/training"
	"github.com/dyoshyy/liftplan/internal/domain/training/condition"
	"github.com/dyoshyy/liftplan/internal/domain/training/exercise"
	"github.com/dyoshyy/liftplan/internal/domain/training/planning"
	"github.com/dyoshyy/liftplan/internal/domain/training/program"
	"github.com/dyoshyy/liftplan/internal/domain/training/seed"
	"github.com/dyoshyy/liftplan/internal/domain/training/setlog"
)

type stubProgram struct {
	prog *program.Program
	err  error
}

func (s *stubProgram) Get(context.Context, account.UserID) (*program.Program, error) {
	return s.prog, s.err
}
func (s *stubProgram) Save(context.Context, account.UserID, *program.Program) error { return nil }

type stubConditions struct {
	log condition.ConditionLog
	err error
}

func (s *stubConditions) FindAll(context.Context, account.UserID) (condition.ConditionLog, error) {
	return s.log, s.err
}

func newProgram(t *testing.T, selected []exercise.ExerciseID) *program.Program {
	t.Helper()
	freq, err := program.NewFrequency(3)
	if err != nil {
		t.Fatalf("頻度が不正: %v", err)
	}
	p, err := program.NewProgram(freq, mustVolume(t, 6, 3), selected, selected, "")
	if err != nil {
		t.Fatalf("プログラムが不正: %v", err)
	}
	return p
}

func newStats(t *testing.T, logs []*setlog.SetLog, pool []*exercise.Exercise, p *program.Program) *query.Stats {
	t.Helper()
	return query.NewStats(
		&stubLogs{history: setlog.NewHistory(logs)},
		&stubExercises{all: pool},
		&stubConditions{},
		&stubProgram{prog: p},
		planning.DefaultOneRepMaxEstimator(),
		planning.DefaultSessionPlanner(),
	)
}

func defaultProgram(t *testing.T) *program.Program {
	t.Helper()
	return newProgram(t, []exercise.ExerciseID{"bench"})
}

// 推移は古い順に並ぶ。並びが崩れると、伸びているのに減っているように見える。
func TestTrends_古い順に並ぶ(t *testing.T) {
	q := newStats(t,
		[]*setlog.SetLog{
			log(t, "c", "2026-08-18", "bench", 100, 5, 1),
			log(t, "a", "2026-08-04", "bench", 90, 5, 1),
			log(t, "b", "2026-08-11", "bench", 95, 5, 1),
		},
		[]*exercise.Exercise{newExercise(t, "bench", "ベンチプレス")},
		defaultProgram(t),
	)

	trends, err := q.Trends(context.Background(), testUser, date(t, "2026-08-01"), date(t, "2026-08-31"))
	if err != nil {
		t.Fatalf("読めない: %v", err)
	}
	if len(trends) != 1 {
		t.Fatalf("種目数が合わない: %d", len(trends))
	}
	tr := trends[0]
	if len(tr.Points) != 3 {
		t.Fatalf("点の数が合わない: %d", len(tr.Points))
	}
	for i := 1; i < len(tr.Points); i++ {
		if !tr.Points[i-1].Date.Before(tr.Points[i].Date) {
			t.Fatalf("古い順に並んでいない: %v", tr.Points)
		}
	}
}

// 現在値と増減は、処方が使う推定1RM（回ごとの値を均したもの）で出す。
//
// 元は現在値を最後の点（その回だけの推定）、増減を最初の点との差にしていた。
// 処方は均した値で重量を決めるので、画面が「デッドリフト 160.3kg」と言う
// 横で処方は約153kgから組まれ、「135kg×3・RIR1 は軽すぎないか」の根拠に
// なっていた（3ヶ月の利用者シミュレーション）。点は回ごとの値のまま残す。
// どの回が高かったか・低かったかは、均すと見えなくなる。
//
// 期待値は、回ごとの推定（Epley: 重量 ×(1 + (回数+RIR)/30)）を古い順に
// alpha 0.3 で均したもの。
//
// 【変異】現在値を最後の点に戻す → 現在値が 120 になって赤。
// 【変異】増減を最初の点との差に戻す → 「期間より前の記録がある」で 12 になって赤。
// 【変異】鮮度の判定を通さない → 「42日より古い」で現在値が出て赤。
func TestTrends_現在値と増減は処方が使う推定で出す(t *testing.T) {
	ptr := func(v float64) *float64 { return &v }

	cases := []struct {
		name        string
		logs        []*setlog.SetLog
		wantCurrent *float64
		wantChange  *float64
	}{
		{
			name: "現在値は最後の点ではなく、均した推定",
			logs: []*setlog.SetLog{
				log(t, "a", "2026-08-04", "bench", 90, 5, 1),  // 108
				log(t, "b", "2026-08-11", "bench", 95, 5, 1),  // 114
				log(t, "c", "2026-08-18", "bench", 100, 5, 1), // 120
			},
			// 108 → 0.3×114 + 0.7×108 = 109.8 → 0.3×120 + 0.7×109.8 = 112.86
			wantCurrent: ptr(112.86),
			// 最初の点の日の推定は 108 そのもの（それより前の記録が無い）
			wantChange: ptr(112.86 - 108),
		},
		{
			// 期間より前の記録も、最初の点の日の推定に効いている。最初の点
			// （その回だけの値）と比べると、均した値と均していない値の差に
			// なり、伸びていない人にも増減が出る。
			name: "増減は最初の点の日の推定との差。期間より前の記録も効く",
			logs: []*setlog.SetLog{
				log(t, "z", "2026-07-28", "bench", 80, 5, 1),  // 96（期間の外）
				log(t, "a", "2026-08-04", "bench", 90, 5, 1),  // 108
				log(t, "b", "2026-08-11", "bench", 95, 5, 1),  // 114
				log(t, "c", "2026-08-18", "bench", 100, 5, 1), // 120
			},
			// 96 → 99.6 → 103.92 → 108.744
			wantCurrent: ptr(108.744),
			// 8/4 時点は 0.3×108 + 0.7×96 = 99.6
			wantChange: ptr(108.744 - 99.6),
		},
		{
			// 最後の記録が42日より前だと、処方は「自分で決める」になる。
			// 画面だけが数字を出すと、処方に無い値を見せることになる。
			name: "42日より古い記録しか無ければ現在値も増減も出さない",
			logs: []*setlog.SetLog{
				log(t, "a", "2026-08-04", "bench", 90, 5, 1),
			},
			wantCurrent: nil,
			wantChange:  nil,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			q := newStats(t, c.logs,
				[]*exercise.Exercise{newExercise(t, "bench", "ベンチプレス")}, defaultProgram(t))

			// 8/4 から 9/16 は43日
			to := "2026-08-31"
			if c.wantCurrent == nil {
				to = "2026-09-16"
			}
			trends, err := q.Trends(context.Background(), testUser, date(t, "2026-08-01"), date(t, to))
			if err != nil {
				t.Fatalf("読めない: %v", err)
			}
			if len(trends) != 1 {
				t.Fatalf("種目数が合わない: %d", len(trends))
			}
			tr := trends[0]
			assertKg(t, "現在値", tr.CurrentKg, c.wantCurrent)
			assertKg(t, "増減", tr.ChangeKg, c.wantChange)
		})
	}
}

func assertKg(t *testing.T, what string, got, want *float64) {
	t.Helper()
	switch {
	case want == nil && got != nil:
		t.Errorf("%sが %v。出さないはず", what, *got)
	case want != nil && got == nil:
		t.Errorf("%sが出ていない。%v のはず", what, *want)
	case want != nil && math.Abs(*got-*want) > 1e-6:
		t.Errorf("%sが %v。%v のはず", what, *got, *want)
	}
}

func TestTrends_期間の外を含めない(t *testing.T) {
	q := newStats(t,
		[]*setlog.SetLog{
			log(t, "a", "2026-07-01", "bench", 90, 5, 1),
			log(t, "b", "2026-08-11", "bench", 95, 5, 1),
		},
		[]*exercise.Exercise{newExercise(t, "bench", "ベンチプレス")},
		defaultProgram(t),
	)

	trends, err := q.Trends(context.Background(), testUser, date(t, "2026-08-01"), date(t, "2026-08-31"))
	if err != nil {
		t.Fatalf("読めない: %v", err)
	}
	if len(trends) != 1 || len(trends[0].Points) != 1 {
		t.Fatalf("期間の外が混ざっている: %+v", trends)
	}
}

// 記録が無い種目は線が引けないので出さない。0 として出すと床に張り付く。
func TestTrends_記録の無い種目は出さない(t *testing.T) {
	q := newStats(t,
		[]*setlog.SetLog{log(t, "a", "2026-08-11", "bench", 95, 5, 1)},
		[]*exercise.Exercise{newExercise(t, "bench", "ベンチプレス"), newExercise(t, "squat", "スクワット")},
		defaultProgram(t),
	)

	trends, err := q.Trends(context.Background(), testUser, date(t, "2026-08-01"), date(t, "2026-08-31"))
	if err != nil {
		t.Fatalf("読めない: %v", err)
	}
	if len(trends) != 1 || trends[0].ExerciseID != "bench" {
		t.Fatalf("記録の無い種目が出ている: %+v", trends)
	}
}

func TestTrends_期間が無ければ断る(t *testing.T) {
	q := newStats(t, nil, nil, defaultProgram(t))
	if _, err := q.Trends(context.Background(), testUser, training.Date{}, date(t, "2026-08-31")); err == nil {
		t.Fatal("期間が無いのに通った")
	}
}

// 週目標に対する充足。埋まっていない区分が上に来る。
// 補助種目が選ばれる理由がここで見える。
func TestWeeklyVolume_埋まっていない順に並ぶ(t *testing.T) {
	bench := newExercise(t, "bench", "ベンチプレス")
	q := newStats(t,
		[]*setlog.SetLog{
			log(t, "a", "2026-08-18", "bench", 100, 5, 1),
			log(t, "b", "2026-08-18", "bench", 100, 5, 1),
		},
		[]*exercise.Exercise{bench},
		defaultProgram(t),
	)

	// 直近4週（当日を含む28日）を数え、週あたりに直す。
	vols, err := q.WeeklyVolume(context.Background(), testUser, date(t, "2026-08-18"))
	if err != nil {
		t.Fatalf("読めない: %v", err)
	}
	// 週目標は設定から組み直すので、全区分が並ぶ。
	if len(vols) != len(training.AllMuscleRegions()) {
		t.Fatalf("区分の数が合わない: %d", len(vols))
	}
	if vols[0].DoneSet > vols[1].DoneSet {
		t.Fatalf("埋まっている方が先頭に来ている: %+v", vols)
	}
	// 胸（中部）に2セット入っている。窓は4週なので、週あたりでは 2 ÷ 4。
	var chest *query.RegionVolume
	for i := range vols {
		if vols[i].Region == training.ChestMid {
			chest = &vols[i]
		}
	}
	if chest == nil {
		t.Fatal("胸（中部）が出ない")
	}
	if want := 2.0 / planning.CoverageWindowWeeks; chest.DoneSet != want {
		t.Fatalf("こなしたセット数（週あたり）が %v。%v のはず", chest.DoneSet, want)
	}
	p := defaultProgram(t)
	want, err := seed.DefaultWeeklyTarget(p.Frequency(), p.SessionVolume())
	if err != nil {
		t.Fatalf("既定の週目標が組めない: %v", err)
	}
	if chest.TargetSet != want.Sets(training.ChestMid) {
		t.Fatalf("週目標が %v。設定から組み直した %v のはず", chest.TargetSet, want.Sets(training.ChestMid))
	}
}

// 窓（当日を含む28日）より前の記録は充足に入らないこと。
//
// 窓はエンジンと同じ長さ。画面だけ長いと、エンジンがもう数えていない
// 記録で埋まって見え、「足りていない区分から選ばれる」が画面の上で
// 成り立たなくなる。2026-07-21 は 2026-08-18 のちょうど28日前。
func TestWeeklyVolume_窓の外を含めない(t *testing.T) {
	q := newStats(t,
		[]*setlog.SetLog{
			log(t, "out", "2026-07-21", "bench", 100, 5, 1), // 28日前。窓の外
			log(t, "in", "2026-07-22", "bench", 100, 5, 1),  // 27日前。窓の内側の端
		},
		[]*exercise.Exercise{newExercise(t, "bench", "ベンチプレス")},
		defaultProgram(t),
	)

	vols, err := q.WeeklyVolume(context.Background(), testUser, date(t, "2026-08-18"))
	if err != nil {
		t.Fatalf("読めない: %v", err)
	}
	// 窓の中の1セットだけが数えられ、週あたりに直る。
	want := 1.0 / planning.CoverageWindowWeeks
	for _, v := range vols {
		if v.Region == training.ChestMid && v.DoneSet != want {
			t.Fatalf("胸（中部）が %v。窓の中の1セットだけで %v のはず", v.DoneSet, want)
		}
	}
}

func TestWeeklyVolume_プログラムが無ければ断る(t *testing.T) {
	q := query.NewStats(
		&stubLogs{history: setlog.NewHistory(nil)},
		&stubExercises{},
		&stubConditions{},
		&stubProgram{prog: nil},
		planning.DefaultOneRepMaxEstimator(),
		planning.DefaultSessionPlanner(),
	)
	if _, err := q.WeeklyVolume(context.Background(), testUser, date(t, "2026-08-18")); err == nil {
		t.Fatal("プログラム未設定なのに通った")
	}
}

// mustVolume はテスト用の1回の量。
func mustVolume(t *testing.T, exercises, sets int) program.SessionVolume {
	t.Helper()
	v, err := program.NewSessionVolume(exercises, sets)
	if err != nil {
		t.Fatalf("NewSessionVolume(%d, %d): %v", exercises, sets, err)
	}
	return v
}

// TestWeeklyVolume_設定から組み直した週目標と比べる は無くなった。
//
// このテストは「保存された週目標が既定と違っていても、画面はそれを無視して
// 設定から組み直した値を見る」ことを、わざと既定と違う保存値（胸中部だけ
// 30）を持つプログラムで確かめていた。Program はもう週目標を保持しない
// （#176）ので、「既定と違う保存値」というプログラム自体が作れなくなり、
// 検査する対象が無くなった。組み直しそのもの（Frequency・SessionVolume
// から導く）は TestWeeklyVolume_埋まっていない順に並ぶ が同じ形で見ている。

// 自重種目の推移は、体重込みの負荷で推定した線になる。
//
// 記録に入っているのは加重だけ（自重でやれば 0kg）。体重を足さずに推定すると
// 全セットが推定できないセッションになり、懸垂を自重で3年続けても線が
// 引かれない（#67）。処方（SessionPlanner）は体重込みで推定しているので、
// 推移も同じ読み替えを通らないと、処方の根拠と画面の線が食い違う。
//
// 期待値は式ではなく「同じ実効負荷を実際に上げた通常種目の線」で与える。
// 推定式（Epley など）が変わっても、この対応は変わらないため。
func TestTrends_自重種目は体重込みの負荷で推定する(t *testing.T) {
	const factor = 0.95 // chinning の自重係数（体重のうち持ち上げる割合）

	cases := []struct {
		name       string
		bodyWeight map[string]float64 // 日付 → 体重
		chin       []struct {
			day     string
			addedKg float64
		}
		// 参照線：通常種目で、その日の実効負荷をそのまま上げたことにする
		effectiveKg []float64
	}{
		{
			name:       "自重のままでも体重ぶんの負荷で点が出る",
			bodyWeight: map[string]float64{"2026-08-01": 70},
			chin: []struct {
				day     string
				addedKg float64
			}{{"2026-08-11", 0}},
			// 0.95 × 70
			effectiveKg: []float64{66.5},
		},
		{
			name:       "加重は体重の上に足される",
			bodyWeight: map[string]float64{"2026-08-01": 70},
			chin: []struct {
				day     string
				addedKg float64
			}{{"2026-08-11", 10}},
			// 10 + 0.95 × 70
			effectiveKg: []float64{76.5},
		},
		{
			// その日の体重で読み替える。減量して同じ回数ができたなら、
			// 実効負荷は下がっている。最新の体重で全部を読み替えると、
			// 過去の点が動いて推移が嘘になる。
			name:       "体重はセッションの日付時点のものを使う",
			bodyWeight: map[string]float64{"2026-08-01": 80, "2026-08-15": 70},
			chin: []struct {
				day     string
				addedKg float64
			}{{"2026-08-11", 0}, {"2026-08-18", 0}},
			// 0.95 × 80 と 0.95 × 70
			effectiveKg: []float64{76, 66.5},
		},
		{
			// 体重を一度も測っていなくても線は引く。処方も既定体重で出す
			// ので、そろえないと処方の根拠が画面に出ない。
			name: "体重を一度も記録していなければ既定体重で読み替える",
			chin: []struct {
				day     string
				addedKg float64
			}{{"2026-08-11", 0}},
			// 0.95 × 既定体重
			effectiveKg: []float64{factor * condition.DefaultBodyWeightKg},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var logs []*setlog.SetLog
			for i, l := range c.chin {
				logs = append(logs,
					log(t, "chin"+l.day, l.day, "chin", l.addedKg, 5, 1),
					log(t, "ref"+l.day, l.day, "ref", c.effectiveKg[i], 5, 1))
			}
			var daily []condition.DailyCondition
			for day, kg := range c.bodyWeight {
				daily = append(daily, condition.NewDailyCondition(date(t, day)).WithBodyWeight(kg))
			}

			pool := []*exercise.Exercise{
				newBodyweightExercise(t, "chin", "チンニング", factor),
				newExercise(t, "ref", "参照"),
			}
			q := query.NewStats(
				&stubLogs{history: setlog.NewHistory(logs)},
				&stubExercises{all: pool},
				&stubConditions{log: condition.NewConditionLog(daily)},
				&stubProgram{prog: newProgram(t, []exercise.ExerciseID{"chin", "ref"})},
				planning.DefaultOneRepMaxEstimator(),
				planning.DefaultSessionPlanner(),
			)

			trends, err := q.Trends(context.Background(), testUser, date(t, "2026-08-01"), date(t, "2026-08-31"))
			if err != nil {
				t.Fatalf("読めない: %v", err)
			}
			byID := map[exercise.ExerciseID]query.Trend{}
			for _, tr := range trends {
				byID[tr.ExerciseID] = tr
			}
			chin, ref := byID["chin"], byID["ref"]
			if len(chin.Points) != len(ref.Points) || len(ref.Points) != len(c.chin) {
				t.Fatalf("点の数が合わない。自重 %d・参照 %d・期待 %d",
					len(chin.Points), len(ref.Points), len(c.chin))
			}
			for i := range ref.Points {
				if math.Abs(chin.Points[i].Kg-ref.Points[i].Kg) > 0.1 {
					t.Errorf("%d点目が %.1fkg。実効負荷 %.1fkg の通常種目と同じ %.1fkg のはず",
						i+1, chin.Points[i].Kg, c.effectiveKg[i], ref.Points[i].Kg)
				}
			}
			// 現在値も同じ読み替えを通る。点だけ体重込みで、現在値を加重の
			// まま推定すると、自重でやっている人の現在値が出なくなる。
			// 【変異】現在値の推定に体重込みの履歴を渡さない → 赤。
			if chin.CurrentKg == nil || ref.CurrentKg == nil {
				t.Fatalf("現在値が出ていない。自重 %v・参照 %v", chin.CurrentKg, ref.CurrentKg)
			}
			if math.Abs(*chin.CurrentKg-*ref.CurrentKg) > 0.1 {
				t.Errorf("現在値が %.1fkg。通常種目と同じ %.1fkg のはず", *chin.CurrentKg, *ref.CurrentKg)
			}
		})
	}
}

// 体重は記録した日だけを古い順に返す。期間の外と、体重の無い日（睡眠だけ
// 記録した日）は含めない。推移の線に「記録していない日」の点が混ざると、
// 測っていない日が0や前日と同じ値に見える。
//
// 【変異】BodyWeights の期間の判定を外す → 8/1 と 8/20 が混ざって赤。
// 【変異】体重の有無を見ない → 8/11 が 0kg の点として混ざって赤。
func TestBodyWeights_期間内の記録した日だけを古い順に返す(t *testing.T) {
	log := condition.NewConditionLog([]condition.DailyCondition{
		condition.NewDailyCondition(date(t, "2026-08-01")).WithBodyWeight(71),
		condition.NewDailyCondition(date(t, "2026-08-12")).WithBodyWeight(70.4),
		condition.NewDailyCondition(date(t, "2026-08-10")).WithBodyWeight(70.8),
		condition.NewDailyCondition(date(t, "2026-08-11")).WithSleepHours(7),
		condition.NewDailyCondition(date(t, "2026-08-20")).WithBodyWeight(69.9),
	})
	q := query.NewStats(
		&stubLogs{history: setlog.NewHistory(nil)},
		&stubExercises{},
		&stubConditions{log: log},
		&stubProgram{prog: defaultProgram(t)},
		planning.DefaultOneRepMaxEstimator(),
		planning.DefaultSessionPlanner(),
	)

	got, err := q.BodyWeights(context.Background(), testUser, date(t, "2026-08-05"), date(t, "2026-08-15"))
	if err != nil {
		t.Fatalf("BodyWeights: %v", err)
	}
	want := []query.Point{{Date: date(t, "2026-08-10"), Kg: 70.8}, {Date: date(t, "2026-08-12"), Kg: 70.4}}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if !got[i].Date.Equal(want[i].Date) || got[i].Kg != want[i].Kg {
			t.Errorf("[%d] = %v, want %v", i, got[i], want[i])
		}
	}
}

func TestBodyWeights_期間が無ければ断る(t *testing.T) {
	q := newStats(t, nil, nil, defaultProgram(t))
	if _, err := q.BodyWeights(context.Background(), testUser, training.Date{}, date(t, "2026-08-15")); err == nil {
		t.Error("期間無しが通ってしまう")
	}
}
