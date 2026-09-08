// Package seed はアプリ同梱の初期データを提供する。
//
// 「メニュー設定が面倒」から始まったのに、自動化を強くするほど初期登録という
// 別の面倒が生まれる。それを潰すのがこのパッケージの役割で、ユーザーがやるのは
// 「使う種目にチェックを入れる」だけにする。
//
// ここに書いた種目リストは出発点であり網羅ではない。
package seed

import "github.com/dyoshyy/liftplan-server/internal/domain/training"

type stimulus = map[training.MuscleRegion]float64

// DefaultDeclared は初期状態で「伸ばしたい種目」に入るものを返す。
//
// 種目マスタの属性ではなく初期設定の提案なので、seed が持つ。本人が
// 設定画面で入れ替える前提で、外れていても壊れない。
//
// 移行前に Kind == KindMain だった3つと同じ顔ぶれ。ここを変えると、
// 新規のユーザーに出るメニューが変わり、シミュレーションの数字も動く。
func DefaultDeclared() []training.ExerciseID {
	return []training.ExerciseID{"bench", "squat", "deadlift"}
}

// exercise は種目1件を組み立てる。
//
// 以前は mainLift と accessory の2つに分かれていた。メイン/補助は種目
// マスタの属性ではなく利用者の目標だったので、Program.declared へ移った
// （D-117）。マスタから見れば、どれも同じ「種目」でしかない。
func exercise(id, name string, inc float64, s stimulus) training.ExerciseParams {
	return training.ExerciseParams{
		ID: id, Name: name,
		Stimulus: s, IncrementKg: inc,
	}
}

// bodyweightAccessory は自重が負荷に乗る補助種目。
//
// 係数は力学的な正確さを狙っていない。推定1RMは伸びを測るための相対値なので、
// 時間を通じて一貫していれば足りる。絶対値がずれても、同じ係数で測り続ける
// 限り推移は正しく出る。
func bodyweightExercise(id, name string, inc, factor float64, s stimulus) training.ExerciseParams {
	p := exercise(id, name, inc, s)
	p.BodyweightFactor = factor
	return p
}

func specs() []training.ExerciseParams {
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

	return []training.ExerciseParams{
		// --- メイン ---
		exercise("squat", "スクワット", 2.5,
			stimulus{r.Quad: 1.0, r.Glute: 0.7, r.Adductor: 0.4, r.Erector: 0.4}),
		exercise("bench", "ベンチプレス", 2.5,
			stimulus{r.ChestMid: 1.0, r.TricepsLateral: 0.5, r.FrontDelt: 0.5}),
		exercise("deadlift", "デッドリフト", 5.0,
			stimulus{r.Hamstring: 1.0, r.Glute: 0.8, r.Erector: 1.0, r.TrapMid: 0.4, r.Forearm: 0.4}),

		// --- メインの派生（補助として残差を埋める） ---
		exercise("larsen_press", "ラーセンプレス", 2.5,
			stimulus{r.ChestMid: 1.0, r.TricepsLateral: 0.5, r.FrontDelt: 0.4}),
		exercise("tempo_bench", "テンポベンチ", 2.5,
			stimulus{r.ChestMid: 1.0, r.TricepsLateral: 0.5, r.FrontDelt: 0.4}),
		exercise("close_grip_bench", "ナローベンチ", 2.5,
			stimulus{r.ChestMid: 0.7, r.TricepsLateral: 1.0, r.TricepsLong: 0.6}),
		exercise("pause_squat", "ポーズスクワット", 2.5,
			stimulus{r.Quad: 1.0, r.Glute: 0.7, r.Adductor: 0.4, r.Erector: 0.4}),
		exercise("front_squat", "フロントスクワット", 2.5,
			stimulus{r.Quad: 1.0, r.Glute: 0.4, r.Erector: 0.5, r.Abs: 0.4}),
		exercise("deficit_deadlift", "デフィシットデッドリフト", 5.0,
			stimulus{r.Hamstring: 1.0, r.Glute: 0.8, r.Erector: 1.0, r.Quad: 0.4}),
		exercise("romanian_deadlift", "ルーマニアンデッドリフト", 2.5,
			stimulus{r.Hamstring: 1.0, r.Glute: 0.7, r.Erector: 0.7}),

		// --- 胸 ---
		exercise("incline_db_press", "インクラインダンベルプレス", 2.0,
			stimulus{r.ChestUpper: 1.0, r.FrontDelt: 0.5, r.TricepsLateral: 0.3}),
		exercise("incline_barbell_press", "インクラインベンチプレス", 2.5,
			stimulus{r.ChestUpper: 1.0, r.FrontDelt: 0.5, r.TricepsLateral: 0.3}),
		bodyweightExercise("dip", "ディップス", 2.5, 0.93,
			stimulus{r.ChestLower: 1.0, r.TricepsLateral: 0.6, r.TricepsLong: 0.4}),
		exercise("decline_press", "デクラインプレス", 2.5,
			stimulus{r.ChestLower: 1.0, r.TricepsLateral: 0.4}),
		exercise("pec_fly", "ペックフライ", 2.5,
			stimulus{r.ChestMid: 1.0, r.ChestUpper: 0.3}),

		// --- 背中 ---
		exercise("lat_pulldown", "ラットプルダウン", 2.5,
			stimulus{r.Lat: 1.0, r.Biceps: 0.4, r.RearDelt: 0.2}),
		bodyweightExercise("pull_up", "チンニング", 2.5, 0.95,
			stimulus{r.Lat: 1.0, r.Biceps: 0.5, r.Forearm: 0.3}),
		exercise("barbell_row", "バーベルロウ", 2.5,
			stimulus{r.Lat: 0.7, r.TrapMid: 1.0, r.RearDelt: 0.4, r.Biceps: 0.3}),
		exercise("seated_row", "シーテッドロウ", 2.5,
			stimulus{r.TrapMid: 1.0, r.Lat: 0.6, r.Biceps: 0.3}),
		bodyweightExercise("back_extension", "バックエクステンション", 2.5, 0.55,
			stimulus{r.Erector: 1.0, r.Glute: 0.5, r.Hamstring: 0.4}),
		exercise("shrug", "シュラッグ", 2.5,
			stimulus{r.TrapUpper: 1.0, r.Forearm: 0.3}),

		// --- 肩 ---
		exercise("overhead_press", "オーバーヘッドプレス", 2.5,
			stimulus{r.FrontDelt: 1.0, r.SideDelt: 0.5, r.TricepsLateral: 0.4}),
		exercise("side_raise", "サイドレイズ", 1.0,
			stimulus{r.SideDelt: 1.0}),
		exercise("rear_delt_fly", "リアデルトフライ", 1.0,
			stimulus{r.RearDelt: 1.0, r.TrapMid: 0.3}),

		// --- 腕 ---
		exercise("triceps_pushdown", "トライセプスプレスダウン", 2.5,
			stimulus{r.TricepsLateral: 1.0, r.TricepsLong: 0.4}),
		exercise("overhead_extension", "オーバーヘッドエクステンション", 2.5,
			stimulus{r.TricepsLong: 1.0, r.TricepsLateral: 0.4}),
		exercise("barbell_curl", "バーベルカール", 2.5,
			stimulus{r.Biceps: 1.0, r.Forearm: 0.4}),
		exercise("hammer_curl", "ハンマーカール", 2.0,
			stimulus{r.Biceps: 0.8, r.Forearm: 1.0}),

		// --- 脚 ---
		exercise("leg_press", "レッグプレス", 5.0,
			stimulus{r.Quad: 1.0, r.Glute: 0.5, r.Adductor: 0.3}),
		exercise("leg_extension", "レッグエクステンション", 2.5,
			stimulus{r.Quad: 1.0}),
		exercise("leg_curl", "レッグカール", 2.5,
			stimulus{r.Hamstring: 1.0}),
		exercise("hip_thrust", "ヒップスラスト", 5.0,
			stimulus{r.Glute: 1.0, r.Hamstring: 0.4}),
		exercise("adductor_machine", "アダクション", 2.5,
			stimulus{r.Adductor: 1.0}),
		exercise("calf_raise", "カーフレイズ", 2.5,
			stimulus{r.Calf: 1.0}),

		// --- 体幹 ---
		exercise("cable_crunch", "ケーブルクランチ", 2.5,
			stimulus{r.Abs: 1.0, r.Oblique: 0.3}),
		exercise("side_bend", "サイドベンド", 2.5,
			stimulus{r.Oblique: 1.0, r.Abs: 0.3}),
	}
}

// Exercises はアプリ同梱の種目マスタ。
func Exercises() ([]*training.Exercise, error) {
	all := specs()
	out := make([]*training.Exercise, 0, len(all))
	for _, p := range all {
		e, err := training.NewExercise(p)
		if err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, nil
}
