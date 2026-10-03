// Package seed はアプリ同梱の初期データを提供する。
//
// 「メニュー設定が面倒」から始まったのに、自動化を強くするほど初期登録という
// 別の面倒が生まれる。それを潰すのがこのパッケージの役割で、ユーザーがやるのは
// 「使う種目にチェックを入れる」だけにする。
//
// ここに書いた種目リストは出発点であり網羅ではない。
package seed

import (
	"github.com/dyoshyy/liftplan/internal/domain/training"
	"github.com/dyoshyy/liftplan/internal/domain/training/exercise"
)

type stimulus = map[training.MuscleRegion]float64

// DefaultDeclared は初期状態で「伸ばしたい種目」に入るものを返す。
//
// 種目マスタの属性ではなく初期設定の提案なので、seed が持つ。本人が
// 設定画面で入れ替える前提で、外れていても壊れない。
//
// 移行前に Kind == KindMain だった3つと同じ顔ぶれ。ここを変えると、
// 新規のユーザーに出るメニューが変わり、シミュレーションの数字も動く。
func DefaultDeclared() []exercise.ExerciseID {
	return []exercise.ExerciseID{"bench", "squat", "deadlift"}
}

// DefaultSelected は新規の利用者が最初から使う種目（20種目）。
//
// 種目は多いほど選択肢が増えるが、最初から全部使うと、使わない種目が一覧に
// 並ぶ。ここに無い種目は、種目の管理で「使う」にするまで計画にも「種目を選んで
// 記録」にも出ない。**既存の利用者の設定は変わらない**（この一覧が効くのは
// 初期プログラムを作るときだけ）。
//
// **減らしすぎると計画が成り立たない。**11種目（BIG3＋各区分に寄与する補助）
// では、全身法でも3〜7区分が達成率の帯（60〜145%）の外に出て、five_way では
// 0セットの日が出た（空の日は周期が止まる）。補助の割り振りが選べる種目が
// 足りないため。そこで、帯の外と空の日が最も減る種目を1つずつ足し、全構成
// （全身法・upper_lower・ppl・five_way × 週2〜7回）で両方が0になるまで
// 足して決めた。顔ぶれの根拠は TestDefaultSelected_PlansEveryShippedSetup。
//
// **シュラッグは入れない**（本人の判断）。僧帽筋上部に寄与する種目は、元の
// カタログではシュラッグだけなので、これが無いと僧帽筋上部の達成率は0%のまま
// になる。受け入れ条件からは僧帽筋上部だけ除いてある。使うなら種目の管理で
// 「使う」にする。ナローベンチも入れず、代わりにトライセプスプレスダウンなどが
// 要った。
//
// 貪欲に足したので、バリエーション（ポーズスクワット・ラーセンプレス）や
// インクラインの2種が入っている。一覧を手で整えるなら、このテストが通ることを
// 条件にする。
func DefaultSelected() []exercise.ExerciseID {
	return []exercise.ExerciseID{
		// 宣言（BIG3）
		"bench", "squat", "deadlift",
		// 胸
		"incline_barbell_press", "incline_db_press", "dip", "larsen_press",
		// 背中
		"lat_pulldown", "pull_up", "seated_row",
		// 肩
		"side_raise", "rear_delt_fly", "overhead_press",
		// 腕
		"triceps_pushdown", "barbell_curl",
		// 脚
		"pause_squat", "romanian_deadlift", "calf_raise",
		// 体幹
		"cable_crunch", "side_bend",
	}
}

// spec は種目1件を組み立てる。
//
// 以前は mainLift と accessory の2つに分かれていた。メイン/補助は種目
// マスタの属性ではなく利用者の目標だったので、Program.declared へ移った
// （D-117）。マスタから見れば、どれも同じ「種目」でしかない。
func spec(id, name string, inc float64, s stimulus) exercise.ExerciseParams {
	return exercise.ExerciseParams{
		ID: id, Name: name,
		Stimulus: s, IncrementKg: inc,
	}
}

// bodyweightAccessory は自重が負荷に乗る補助種目。
//
// 係数は力学的な正確さを狙っていない。推定1RMは伸びを測るための相対値なので、
// 時間を通じて一貫していれば足りる。絶対値がずれても、同じ係数で測り続ける
// 限り推移は正しく出る。
func bodyweightExercise(id, name string, inc, factor float64, s stimulus) exercise.ExerciseParams {
	p := spec(id, name, inc, s)
	p.BodyweightFactor = factor
	return p
}

// derived は別の種目から派生した種目を組み立てる。
//
// 派生は重点種目のバリエーションとして回る。親そのものではなく、同じ動作の
// 変種（ポーズを入れる、握りを変える、可動域を変える）を指す。
//
// 重量は親から換算しない。派生は自分の記録から自分の推定1RMを持つ（D-113）。
// ここが表しているのは「どの種目の変種か」という関係だけで、強度の比では
// ない。
func derived(from, id, name string, inc float64, s stimulus) exercise.ExerciseParams {
	p := spec(id, name, inc, s)
	p.DerivedFrom = from
	return p
}

func specs() []exercise.ExerciseParams {
	r := struct {
		ChestUpper, ChestMid, ChestLower                     training.MuscleRegion
		Lat, TrapMid, TrapUpper, Erector                     training.MuscleRegion
		FrontDelt, SideDelt, RearDelt                        training.MuscleRegion
		TricepsLong, TricepsLateral, Biceps, Forearm         training.MuscleRegion
		Quad, Hamstring, Glute, Adductor, Calf, Abs, Oblique training.MuscleRegion
	}{
		training.ChestUpper, training.ChestMid, training.ChestLower,
		training.Lat, training.TrapMid, training.TrapUpper, training.Erector,
		training.FrontDelt, training.SideDelt, training.RearDelt,
		training.TricepsLong, training.TricepsLateral, training.Biceps, training.Forearm,
		training.Quad, training.Hamstring, training.Glute, training.Adductor,
		training.Calf, training.Abs, training.Oblique,
	}

	base := []exercise.ExerciseParams{
		// --- メイン ---
		spec("squat", "スクワット", 2.5,
			stimulus{r.Quad: 1.0, r.Glute: 0.7, r.Adductor: 0.4, r.Erector: 0.4}),
		spec("bench", "ベンチプレス", 2.5,
			stimulus{r.ChestMid: 1.0, r.TricepsLateral: 0.5, r.FrontDelt: 0.5, r.ChestLower: 0.3, r.ChestUpper: 0.3}),
		spec("deadlift", "デッドリフト", 5.0,
			stimulus{r.Hamstring: 1.0, r.Glute: 0.8, r.Erector: 1.0, r.TrapMid: 0.4, r.Forearm: 0.4, r.Quad: 0.3, r.Lat: 0.3}),

		// --- 派生（重点種目のバリエーションとして回る） ---
		//
		// 補助としても残差を埋める。派生であることと、補助に選ばれることは
		// 別の話で、どの種目の変種かを表しているだけ。
		derived("bench", "larsen_press", "ラーセンプレス", 2.5,
			stimulus{r.ChestMid: 1.0, r.TricepsLateral: 0.5, r.FrontDelt: 0.4, r.ChestLower: 0.3, r.ChestUpper: 0.3}),
		derived("bench", "tempo_bench", "テンポベンチ", 2.5,
			stimulus{r.ChestMid: 1.0, r.TricepsLateral: 0.5, r.FrontDelt: 0.4, r.ChestLower: 0.3, r.ChestUpper: 0.3}),
		derived("bench", "close_grip_bench", "ナローベンチ", 2.5,
			stimulus{r.ChestMid: 0.7, r.TricepsLateral: 1.0, r.TricepsLong: 0.6}),
		derived("squat", "pause_squat", "ポーズスクワット", 2.5,
			stimulus{r.Quad: 1.0, r.Glute: 0.7, r.Adductor: 0.4, r.Erector: 0.4}),
		derived("squat", "front_squat", "フロントスクワット", 2.5,
			stimulus{r.Quad: 1.0, r.Glute: 0.4, r.Erector: 0.5, r.Abs: 0.4}),
		derived("deadlift", "deficit_deadlift", "デフィシットデッドリフト", 5.0,
			stimulus{r.Hamstring: 1.0, r.Glute: 0.8, r.Erector: 1.0, r.Quad: 0.4, r.Lat: 0.3}),
		// RDL は宣言（軸レーン）にも入りうる。系統に属することと、伸ばしたい
		// 種目として宣言することは別の軸（2026-09-10 の仕様）。
		derived("deadlift", "romanian_deadlift", "ルーマニアンデッドリフト", 2.5,
			stimulus{r.Hamstring: 1.0, r.Glute: 0.7, r.Erector: 0.7}),

		// --- 胸 ---
		spec("incline_db_press", "インクラインダンベルプレス", 2.0,
			stimulus{r.ChestUpper: 1.0, r.FrontDelt: 0.5, r.TricepsLateral: 0.3}),
		spec("incline_barbell_press", "インクラインベンチプレス", 2.5,
			stimulus{r.ChestUpper: 1.0, r.FrontDelt: 0.5, r.TricepsLateral: 0.3}),
		bodyweightExercise("dip", "ディップス", 2.5, 0.93,
			stimulus{r.ChestLower: 1.0, r.TricepsLateral: 0.6, r.TricepsLong: 0.4, r.FrontDelt: 0.4, r.ChestMid: 0.4}),
		spec("decline_press", "デクラインプレス", 2.5,
			stimulus{r.ChestLower: 1.0, r.TricepsLateral: 0.4}),
		spec("pec_fly", "ペックフライ", 2.5,
			stimulus{r.ChestMid: 1.0, r.ChestUpper: 0.3}),

		// --- 背中 ---
		spec("lat_pulldown", "ラットプルダウン", 2.5,
			stimulus{r.Lat: 1.0, r.Biceps: 0.4, r.RearDelt: 0.2, r.Forearm: 0.2, r.TrapMid: 0.2}),
		bodyweightExercise("pull_up", "チンニング", 2.5, 0.95,
			stimulus{r.Lat: 1.0, r.Biceps: 0.5, r.Forearm: 0.3, r.RearDelt: 0.2, r.TrapMid: 0.2}),
		spec("barbell_row", "バーベルロウ", 2.5,
			stimulus{r.Lat: 0.7, r.TrapMid: 1.0, r.RearDelt: 0.4, r.Biceps: 0.3, r.TrapUpper: 0.3, r.Forearm: 0.3}),
		spec("seated_row", "シーテッドロウ", 2.5,
			stimulus{r.TrapMid: 1.0, r.Lat: 0.6, r.Biceps: 0.3, r.TrapUpper: 0.2, r.RearDelt: 0.4, r.Forearm: 0.3}),
		bodyweightExercise("back_extension", "バックエクステンション", 2.5, 0.55,
			stimulus{r.Erector: 1.0, r.Glute: 0.5, r.Hamstring: 0.4}),
		spec("shrug", "シュラッグ", 2.5,
			stimulus{r.TrapUpper: 1.0, r.Forearm: 0.3}),

		// --- 肩 ---
		spec("overhead_press", "オーバーヘッドプレス", 2.5,
			stimulus{r.FrontDelt: 1.0, r.SideDelt: 0.5, r.TricepsLateral: 0.4, r.TrapUpper: 0.3, r.ChestUpper: 0.3}),
		spec("db_shoulder_press", "ダンベルショルダープレス", 2.0,
			stimulus{r.FrontDelt: 1.0, r.SideDelt: 0.5, r.TricepsLateral: 0.4, r.TrapUpper: 0.3}),
		spec("side_raise", "サイドレイズ", 1.0,
			stimulus{r.SideDelt: 1.0, r.TrapUpper: 0.3}),
		spec("cable_side_raise", "ケーブルサイドレイズ", 2.5,
			stimulus{r.SideDelt: 1.0, r.TrapUpper: 0.3}),
		spec("rear_delt_fly", "リアデルトフライ", 1.0,
			stimulus{r.RearDelt: 1.0, r.TrapMid: 0.3, r.TrapUpper: 0.3}),

		// --- 腕 ---
		spec("triceps_pushdown", "トライセプスプレスダウン", 2.5,
			stimulus{r.TricepsLateral: 1.0, r.TricepsLong: 0.4}),
		spec("overhead_extension", "オーバーヘッドエクステンション", 2.5,
			stimulus{r.TricepsLong: 1.0, r.TricepsLateral: 0.4}),
		spec("barbell_curl", "バーベルカール", 2.5,
			stimulus{r.Biceps: 1.0, r.Forearm: 0.4}),
		spec("hammer_curl", "ハンマーカール", 2.0,
			stimulus{r.Biceps: 0.8, r.Forearm: 1.0}),

		// --- 脚 ---
		spec("leg_press", "レッグプレス", 5.0,
			stimulus{r.Quad: 1.0, r.Glute: 0.5, r.Adductor: 0.3}),
		spec("leg_extension", "レッグエクステンション", 2.5,
			stimulus{r.Quad: 1.0}),
		spec("leg_curl", "レッグカール", 2.5,
			stimulus{r.Hamstring: 1.0}),
		spec("hip_thrust", "ヒップスラスト", 5.0,
			stimulus{r.Glute: 1.0, r.Hamstring: 0.4}),
		spec("adductor_machine", "アダクション", 2.5,
			stimulus{r.Adductor: 1.0}),
		spec("calf_raise", "カーフレイズ", 2.5,
			stimulus{r.Calf: 1.0}),

		// --- 体幹 ---
		spec("cable_crunch", "ケーブルクランチ", 2.5,
			stimulus{r.Abs: 1.0, r.Oblique: 0.3}),
		spec("side_bend", "サイドベンド", 2.5,
			stimulus{r.Oblique: 1.0, r.Abs: 0.3}),
	}
	return append(base, hammerStrengthSpecs()...)
}

// Exercises はアプリ同梱の種目マスタ。
func Exercises() ([]*exercise.Exercise, error) {
	all := specs()
	out := make([]*exercise.Exercise, 0, len(all))
	for _, p := range all {
		e, err := exercise.NewExercise(p)
		if err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, nil
}
