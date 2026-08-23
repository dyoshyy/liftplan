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

func mainLift(id, name string, lift training.MainLift, inc float64, s stimulus) training.ExerciseParams {
	return training.ExerciseParams{
		ID: id, Name: name, Kind: training.KindMain,
		Stimulus: s, IncrementKg: inc, MainLift: lift,
	}
}

func variation(id, name string, lift training.MainLift, inc float64, s stimulus) training.ExerciseParams {
	return training.ExerciseParams{
		ID: id, Name: name, Kind: training.KindVariation,
		Stimulus: s, IncrementKg: inc, MainLift: lift,
	}
}

func accessory(id, name string, inc float64, s stimulus) training.ExerciseParams {
	return training.ExerciseParams{
		ID: id, Name: name, Kind: training.KindAccessory,
		Stimulus: s, IncrementKg: inc,
	}
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
		mainLift("squat", "スクワット", training.LiftSquat, 2.5,
			stimulus{r.Quad: 1.0, r.Glute: 0.7, r.Adductor: 0.4, r.Erector: 0.4}),
		mainLift("bench", "ベンチプレス", training.LiftBench, 2.5,
			stimulus{r.ChestMid: 1.0, r.TricepsLateral: 0.5, r.FrontDelt: 0.5}),
		mainLift("deadlift", "デッドリフト", training.LiftDeadlift, 5.0,
			stimulus{r.Hamstring: 1.0, r.Glute: 0.8, r.Erector: 1.0, r.TrapMid: 0.4, r.Forearm: 0.4}),

		// --- バリエーション ---
		variation("larsen_press", "ラーセンプレス", training.LiftBench, 2.5,
			stimulus{r.ChestMid: 1.0, r.TricepsLateral: 0.5, r.FrontDelt: 0.4}),
		variation("tempo_bench", "テンポベンチ", training.LiftBench, 2.5,
			stimulus{r.ChestMid: 1.0, r.TricepsLateral: 0.5, r.FrontDelt: 0.4}),
		variation("close_grip_bench", "ナローベンチ", training.LiftBench, 2.5,
			stimulus{r.ChestMid: 0.7, r.TricepsLateral: 1.0, r.TricepsLong: 0.6}),
		variation("pause_squat", "ポーズスクワット", training.LiftSquat, 2.5,
			stimulus{r.Quad: 1.0, r.Glute: 0.7, r.Adductor: 0.4, r.Erector: 0.4}),
		variation("front_squat", "フロントスクワット", training.LiftSquat, 2.5,
			stimulus{r.Quad: 1.0, r.Glute: 0.4, r.Erector: 0.5, r.Abs: 0.4}),
		variation("deficit_deadlift", "デフィシットデッドリフト", training.LiftDeadlift, 5.0,
			stimulus{r.Hamstring: 1.0, r.Glute: 0.8, r.Erector: 1.0, r.Quad: 0.4}),
		variation("romanian_deadlift", "ルーマニアンデッドリフト", training.LiftDeadlift, 2.5,
			stimulus{r.Hamstring: 1.0, r.Glute: 0.7, r.Erector: 0.7}),

		// --- 胸 ---
		accessory("incline_db_press", "インクラインダンベルプレス", 2.0,
			stimulus{r.ChestUpper: 1.0, r.FrontDelt: 0.5, r.TricepsLateral: 0.3}),
		accessory("incline_barbell_press", "インクラインベンチプレス", 2.5,
			stimulus{r.ChestUpper: 1.0, r.FrontDelt: 0.5, r.TricepsLateral: 0.3}),
		accessory("dip", "ディップス", 2.5,
			stimulus{r.ChestLower: 1.0, r.TricepsLateral: 0.6, r.TricepsLong: 0.4}),
		accessory("decline_press", "デクラインプレス", 2.5,
			stimulus{r.ChestLower: 1.0, r.TricepsLateral: 0.4}),
		accessory("pec_fly", "ペックフライ", 2.5,
			stimulus{r.ChestMid: 1.0, r.ChestUpper: 0.3}),

		// --- 背中 ---
		accessory("lat_pulldown", "ラットプルダウン", 2.5,
			stimulus{r.Lat: 1.0, r.Biceps: 0.4, r.RearDelt: 0.2}),
		accessory("pull_up", "チンニング", 2.5,
			stimulus{r.Lat: 1.0, r.Biceps: 0.5, r.Forearm: 0.3}),
		accessory("barbell_row", "バーベルロウ", 2.5,
			stimulus{r.Lat: 0.7, r.TrapMid: 1.0, r.RearDelt: 0.4, r.Biceps: 0.3}),
		accessory("seated_row", "シーテッドロウ", 2.5,
			stimulus{r.TrapMid: 1.0, r.Lat: 0.6, r.Biceps: 0.3}),
		accessory("back_extension", "バックエクステンション", 2.5,
			stimulus{r.Erector: 1.0, r.Glute: 0.5, r.Hamstring: 0.4}),
		accessory("shrug", "シュラッグ", 2.5,
			stimulus{r.TrapUpper: 1.0, r.Forearm: 0.3}),

		// --- 肩 ---
		accessory("overhead_press", "オーバーヘッドプレス", 2.5,
			stimulus{r.FrontDelt: 1.0, r.SideDelt: 0.5, r.TricepsLateral: 0.4}),
		accessory("side_raise", "サイドレイズ", 1.0,
			stimulus{r.SideDelt: 1.0}),
		accessory("rear_delt_fly", "リアデルトフライ", 1.0,
			stimulus{r.RearDelt: 1.0, r.TrapMid: 0.3}),

		// --- 腕 ---
		accessory("triceps_pushdown", "トライセプスプレスダウン", 2.5,
			stimulus{r.TricepsLateral: 1.0, r.TricepsLong: 0.4}),
		accessory("overhead_extension", "オーバーヘッドエクステンション", 2.5,
			stimulus{r.TricepsLong: 1.0, r.TricepsLateral: 0.4}),
		accessory("barbell_curl", "バーベルカール", 2.5,
			stimulus{r.Biceps: 1.0, r.Forearm: 0.4}),
		accessory("hammer_curl", "ハンマーカール", 2.0,
			stimulus{r.Biceps: 0.8, r.Forearm: 1.0}),

		// --- 脚 ---
		accessory("leg_press", "レッグプレス", 5.0,
			stimulus{r.Quad: 1.0, r.Glute: 0.5, r.Adductor: 0.3}),
		accessory("leg_extension", "レッグエクステンション", 2.5,
			stimulus{r.Quad: 1.0}),
		accessory("leg_curl", "レッグカール", 2.5,
			stimulus{r.Hamstring: 1.0}),
		accessory("hip_thrust", "ヒップスラスト", 5.0,
			stimulus{r.Glute: 1.0, r.Hamstring: 0.4}),
		accessory("adductor_machine", "アダクション", 2.5,
			stimulus{r.Adductor: 1.0}),
		accessory("calf_raise", "カーフレイズ", 2.5,
			stimulus{r.Calf: 1.0}),

		// --- 体幹 ---
		accessory("cable_crunch", "ケーブルクランチ", 2.5,
			stimulus{r.Abs: 1.0, r.Oblique: 0.3}),
		accessory("side_bend", "サイドベンド", 2.5,
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
