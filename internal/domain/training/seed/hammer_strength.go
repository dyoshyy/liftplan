package seed

import (
	"github.com/dyoshyy/liftplan/internal/domain/training"
	"github.com/dyoshyy/liftplan/internal/domain/training/exercise"
)

// hammerStrengthSpecs は Hammer Strength のマシンのプリセット。
//
// 出典は Life Fitness の公式カタログ（2026-10-03 時点）の3系列：
//
//   - Plate-Loaded（https://www.lifefitness.com/en-us/catalog/strength-training/plate-loaded）
//   - Motion Technology Selectorized（MTS）
//   - Hammer Strength Select
//
// **1台1種目。**同じ動作でも、系列やマシンが違えば重量の刻みも感触も違い、
// 推定1RMは種目ごとに持つ。名前は「HS ＋ 機種名 ＋（系列）」で、汎用の種目
// （レッグプレスなど）や同じ動作の別系列と見分けがつくようにする。
//
// 入れていないマシン：筋区分に寄与を付けられないもの（4-Way Neck、
// Tibia Dorsi-Flexion）、スミスマシン（Smith Machine・Vertical Smith Machine。
// スクワットにもベンチにも使え、動作が決まらない）、Select の "Outlet" 版
// （標準版と同じ動作）。2通りに使えるマシン（Chest/Back、Pectoral Fly/Rear
// Deltoid）は、使い方ごとに別の種目にした。Assist Dip Chin、Seated/Standing
// Shrug、Glute Ham/Reverse Hyper、Ground Base Squat/High Pull は、主に使う
// 動作の寄与を置いている。
//
// **寄与（効き方）と刻みは仮置き。**アイソラテラル系の9種目は、種目の管理の設計書
// （docs/specs/2026-09-26-custom-exercises-design.md 付録）の初期案どおり。残りは
// 同じ動作の汎用の種目（ベンチプレス・ラットプルダウンなど）の寄与に揃えた推定で、
// 実機の感触で直すものとして、種目の管理から名前・効き方・刻みを直せる。
// 刻みは、脚を押す・しゃがむマシンと臀筋のマシンを 5kg、ほかを 2.5kg にした。
//
// **新規の利用者が最初から使う種目には入れない**（DefaultSelected）。カタログに
// 入っているだけで、種目の管理で「使う」にして初めて計画に出る。既存の利用者には
// 届かない（その人の一覧は、初回の読み出しでコピーした行のまま）。
//
// 寄与は区分名を直接書く（specs の r は関数の中に閉じている）。
func hammerStrengthSpecs() []exercise.ExerciseParams {
	return []exercise.ExerciseParams{
		// --- プレート式・アイソラテラル ---
		spec("hs_pl_iso_incline_press", "HS アイソラテラル・インクライン・プレス（プレート）", 2.5, stimulus{training.ChestUpper: 1.0, training.FrontDelt: 0.5, training.TricepsLateral: 0.5}),
		spec("hs_pl_iso_super_incline_press", "HS アイソラテラル・スーパーインクライン・プレス（プレート）", 2.5, stimulus{training.ChestUpper: 1.0, training.FrontDelt: 0.7, training.TricepsLateral: 0.4}),
		spec("hs_pl_iso_bench_press", "HS アイソラテラル・ベンチ・プレス（プレート）", 2.5, stimulus{training.ChestMid: 1.0, training.TricepsLateral: 0.5, training.FrontDelt: 0.5}),
		spec("hs_pl_iso_horizontal_bench_press", "HS アイソラテラル・ホリゾンタル・ベンチ・プレス（プレート）", 2.5, stimulus{training.ChestMid: 1.0, training.TricepsLateral: 0.5, training.FrontDelt: 0.5}),
		spec("hs_pl_iso_wide_chest", "HS アイソラテラル・ワイド・チェスト（プレート）", 2.5, stimulus{training.ChestMid: 1.0, training.FrontDelt: 0.5, training.TricepsLateral: 0.4}),
		spec("hs_pl_iso_decline_chest_press", "HS アイソラテラル・デクライン・チェスト・プレス（プレート）", 2.5, stimulus{training.ChestLower: 1.0, training.TricepsLateral: 0.5}),
		spec("hs_pl_iso_shoulder_press", "HS アイソラテラル・ショルダー・プレス（プレート）", 2.5, stimulus{training.FrontDelt: 1.0, training.SideDelt: 0.5, training.TricepsLateral: 0.5}),
		spec("hs_pl_iso_row", "HS アイソラテラル・ロー（プレート）", 2.5, stimulus{training.TrapMid: 1.0, training.Lat: 0.5, training.Biceps: 0.5, training.RearDelt: 0.5}),
		spec("hs_pl_iso_high_row", "HS アイソラテラル・ハイ・ロー（プレート）", 2.5, stimulus{training.Lat: 1.0, training.TrapMid: 0.5, training.RearDelt: 0.5, training.Biceps: 0.5}),
		spec("hs_pl_iso_low_row", "HS アイソラテラル・ロー・ロー（プレート）", 2.5, stimulus{training.Lat: 1.0, training.TrapMid: 0.5, training.Biceps: 0.5}),
		spec("hs_pl_iso_dy_row", "HS アイソラテラル・DY・ロー（プレート）", 2.5, stimulus{training.Lat: 1.0, training.TrapMid: 0.5, training.Biceps: 0.5}),
		spec("hs_pl_iso_wide_pulldown", "HS アイソラテラル・ワイド・プルダウン（プレート）", 2.5, stimulus{training.Lat: 1.0, training.TrapMid: 0.5, training.Biceps: 0.5}),
		spec("hs_pl_iso_front_pulldown", "HS アイソラテラル・フロント・プルダウン（プレート）", 2.5, stimulus{training.Lat: 1.0, training.Biceps: 0.5}),
		spec("hs_pl_iso_t_bar_row", "HS アイソラテラル・Tバー・ロー（プレート）", 2.5, stimulus{training.TrapMid: 1.0, training.Lat: 0.8, training.RearDelt: 0.4, training.Biceps: 0.3, training.Erector: 0.3}),
		spec("hs_pl_iso_chest_back_chest", "HS アイソラテラル・チェスト／バック（胸）（プレート）", 2.5, stimulus{training.ChestMid: 1.0, training.FrontDelt: 0.4, training.TricepsLateral: 0.4}),
		spec("hs_pl_iso_chest_back_back", "HS アイソラテラル・チェスト／バック（背中）（プレート）", 2.5, stimulus{training.TrapMid: 1.0, training.Lat: 0.6, training.Biceps: 0.4}),
		spec("hs_pl_iso_leg_extension", "HS アイソラテラル・レッグエクステンション（プレート）", 2.5, stimulus{training.Quad: 1.0}),
		spec("hs_pl_iso_leg_curl", "HS アイソラテラル・レッグカール（プレート）", 2.5, stimulus{training.Hamstring: 1.0}),
		spec("hs_pl_iso_kneeling_leg_curl", "HS アイソラテラル・ニーリング・レッグカール（プレート）", 2.5, stimulus{training.Hamstring: 1.0}),

		// --- プレート式 ---
		spec("hs_pl_incline_press", "HS インクライン・プレス（プレート）", 2.5, stimulus{training.ChestUpper: 1.0, training.FrontDelt: 0.5, training.TricepsLateral: 0.3}),
		spec("hs_pl_decline_chest_press", "HS デクライン・チェスト・プレス（プレート）", 2.5, stimulus{training.ChestLower: 1.0, training.TricepsLateral: 0.4}),
		spec("hs_pl_super_fly", "HS スーパー・フライ（プレート）", 2.5, stimulus{training.ChestMid: 1.0, training.ChestUpper: 0.3}),
		spec("hs_pl_seated_dip", "HS シーテッド・ディップ（プレート）", 2.5, stimulus{training.ChestLower: 1.0, training.TricepsLateral: 0.6, training.TricepsLong: 0.4}),
		spec("hs_pl_pullover", "HS プルオーバー（プレート）", 2.5, stimulus{training.Lat: 1.0, training.TricepsLong: 0.4, training.ChestMid: 0.3}),
		spec("hs_pl_pulldown", "HS プルダウン（プレート）", 2.5, stimulus{training.Lat: 1.0, training.Biceps: 0.4, training.RearDelt: 0.2}),
		spec("hs_pl_high_row", "HS ハイ・ロー（プレート）", 2.5, stimulus{training.Lat: 1.0, training.TrapMid: 0.5, training.RearDelt: 0.3, training.Biceps: 0.3}),
		spec("hs_pl_row", "HS ロー（プレート）", 2.5, stimulus{training.TrapMid: 1.0, training.Lat: 0.6, training.Biceps: 0.3}),
		spec("hs_pl_t_bar_row", "HS Tバー・ロー（プレート）", 2.5, stimulus{training.TrapMid: 1.0, training.Lat: 0.8, training.RearDelt: 0.4, training.Biceps: 0.3, training.Erector: 0.3}),
		spec("hs_pl_shrug", "HS シュラッグ（座り／立ち）（プレート）", 2.5, stimulus{training.TrapUpper: 1.0, training.Forearm: 0.3}),
		spec("hs_pl_shoulder_press", "HS ショルダー・プレス（プレート）", 2.5, stimulus{training.FrontDelt: 1.0, training.SideDelt: 0.5, training.TricepsLateral: 0.4}),
		spec("hs_pl_lateral_raise", "HS ラテラルレイズ（プレート）", 2.5, stimulus{training.SideDelt: 1.0}),
		spec("hs_pl_seated_biceps", "HS シーテッド・バイセップス（プレート）", 2.5, stimulus{training.Biceps: 1.0, training.Forearm: 0.3}),
		spec("hs_pl_gripper", "HS グリッパー（プレート）", 2.5, stimulus{training.Forearm: 1.0}),
		spec("hs_pl_reverse_v_squat", "HS リバースVスクワット（プレート）", 5.0, stimulus{training.Quad: 1.0, training.Glute: 0.7, training.Adductor: 0.3, training.Erector: 0.3}),
		spec("hs_pl_super_squat_press", "HS スーパー・スクワット・プレス（プレート）", 5.0, stimulus{training.Quad: 1.0, training.Glute: 0.6, training.Adductor: 0.3}),
		spec("hs_pl_pendulum_x_squat", "HS ペンデュラムX・スクワット（プレート）", 5.0, stimulus{training.Quad: 1.0, training.Glute: 0.6, training.Adductor: 0.3}),
		spec("hs_pl_ground_base_squat_high_pull", "HS グラウンドベース・スクワット／ハイプル（プレート）", 5.0, stimulus{training.Quad: 1.0, training.Glute: 0.6, training.Erector: 0.4, training.TrapUpper: 0.3}),
		spec("hs_pl_hack_squat", "HS ハックスクワット（プレート）", 5.0, stimulus{training.Quad: 1.0, training.Glute: 0.5, training.Adductor: 0.2}),
		spec("hs_pl_belt_squat", "HS ベルトスクワット（プレート）", 5.0, stimulus{training.Quad: 1.0, training.Glute: 0.6, training.Adductor: 0.3}),
		spec("hs_pl_linear_leg_press", "HS リニア・レッグプレス（プレート）", 5.0, stimulus{training.Quad: 1.0, training.Glute: 0.5, training.Adductor: 0.3}),
		spec("hs_pl_leg_extension", "HS レッグエクステンション（プレート）", 2.5, stimulus{training.Quad: 1.0}),
		spec("hs_pl_kneeling_leg_curl", "HS ニーリング・レッグカール（プレート）", 2.5, stimulus{training.Hamstring: 1.0}),
		spec("hs_pl_assisted_nordic_ham", "HS アシスト・ノルディックハム（プレート）", 2.5, stimulus{training.Hamstring: 1.0, training.Glute: 0.3}),
		spec("hs_pl_glute_ham_reverse_hyper", "HS グルートハム／リバースハイパー（プレート）", 2.5, stimulus{training.Hamstring: 1.0, training.Glute: 0.7, training.Erector: 0.6}),
		spec("hs_pl_glute_drive", "HS グルート・ドライブ（プレート）", 5.0, stimulus{training.Glute: 1.0, training.Hamstring: 0.4}),
		spec("hs_pl_seated_calf_raise", "HS シーテッド・カーフレイズ（プレート）", 2.5, stimulus{training.Calf: 1.0}),
		spec("hs_pl_calf_raise", "HS カーフレイズ（プレート）", 2.5, stimulus{training.Calf: 1.0}),
		spec("hs_pl_abdominal_oblique_crunch", "HS アブドミナル／オブリーク・クランチ（プレート）", 2.5, stimulus{training.Abs: 1.0, training.Oblique: 0.5}),

		// --- MTS ---
		spec("hs_mts_abdominal_crunch", "HS アブドミナル・クランチ（MTS）", 2.5, stimulus{training.Abs: 1.0, training.Oblique: 0.3}),
		spec("hs_mts_iso_biceps_curl", "HS アイソラテラル・バイセップス・カール（MTS）", 2.5, stimulus{training.Biceps: 1.0, training.Forearm: 0.3}),
		spec("hs_mts_iso_chest_press", "HS アイソラテラル・チェスト・プレス（MTS）", 2.5, stimulus{training.ChestMid: 1.0, training.TricepsLateral: 0.5, training.FrontDelt: 0.5}),
		spec("hs_mts_iso_decline_press", "HS アイソラテラル・デクライン・プレス（MTS）", 2.5, stimulus{training.ChestLower: 1.0, training.TricepsLateral: 0.5}),
		spec("hs_mts_iso_front_pulldown", "HS アイソラテラル・フロント・プルダウン（MTS）", 2.5, stimulus{training.Lat: 1.0, training.Biceps: 0.5}),
		spec("hs_mts_iso_high_row", "HS アイソラテラル・ハイ・ロー（MTS）", 2.5, stimulus{training.Lat: 1.0, training.TrapMid: 0.5, training.RearDelt: 0.5, training.Biceps: 0.5}),
		spec("hs_mts_iso_incline_press", "HS アイソラテラル・インクライン・プレス（MTS）", 2.5, stimulus{training.ChestUpper: 1.0, training.FrontDelt: 0.5, training.TricepsLateral: 0.5}),
		spec("hs_mts_iso_row", "HS アイソラテラル・ロー（MTS）", 2.5, stimulus{training.TrapMid: 1.0, training.Lat: 0.5, training.Biceps: 0.5, training.RearDelt: 0.5}),
		spec("hs_mts_iso_shoulder_press", "HS アイソラテラル・ショルダー・プレス（MTS）", 2.5, stimulus{training.FrontDelt: 1.0, training.SideDelt: 0.5, training.TricepsLateral: 0.5}),
		spec("hs_mts_iso_triceps_extension", "HS アイソラテラル・トライセプス・エクステンション（MTS）", 2.5, stimulus{training.TricepsLateral: 1.0, training.TricepsLong: 0.5}),
		spec("hs_mts_iso_leg_extension", "HS アイソラテラル・レッグエクステンション（MTS）", 2.5, stimulus{training.Quad: 1.0}),
		spec("hs_mts_iso_kneeling_leg_curl", "HS アイソラテラル・ニーリング・レッグカール（MTS）", 2.5, stimulus{training.Hamstring: 1.0}),

		// --- セレクト ---
		spec("hs_sel_assist_dip_chin", "HS アシスト・ディップ／チン（セレクト）", 2.5, stimulus{training.Lat: 1.0, training.Biceps: 0.5, training.Forearm: 0.3}),
		spec("hs_sel_chest_press", "HS チェスト・プレス（セレクト）", 2.5, stimulus{training.ChestMid: 1.0, training.TricepsLateral: 0.5, training.FrontDelt: 0.5}),
		spec("hs_sel_pectoral_fly", "HS ペクトラル・フライ（セレクト）", 2.5, stimulus{training.ChestMid: 1.0, training.ChestUpper: 0.3}),
		spec("hs_sel_rear_deltoid", "HS リアデルト（ペクトラル・フライ／リアデルト）（セレクト）", 2.5, stimulus{training.RearDelt: 1.0, training.TrapMid: 0.3}),
		spec("hs_sel_shoulder_press", "HS ショルダー・プレス（セレクト）", 2.5, stimulus{training.FrontDelt: 1.0, training.SideDelt: 0.5, training.TricepsLateral: 0.4}),
		spec("hs_sel_lateral_raise", "HS ラテラルレイズ（セレクト）", 2.5, stimulus{training.SideDelt: 1.0}),
		spec("hs_sel_lat_pulldown", "HS ラット・プルダウン（セレクト）", 2.5, stimulus{training.Lat: 1.0, training.Biceps: 0.4, training.RearDelt: 0.2}),
		spec("hs_sel_seated_row", "HS シーテッド・ロー（セレクト）", 2.5, stimulus{training.TrapMid: 1.0, training.Lat: 0.6, training.Biceps: 0.3}),
		spec("hs_sel_biceps_curl", "HS バイセップス・カール（セレクト）", 2.5, stimulus{training.Biceps: 1.0, training.Forearm: 0.3}),
		spec("hs_sel_triceps_extension", "HS トライセプス・エクステンション（セレクト）", 2.5, stimulus{training.TricepsLateral: 1.0, training.TricepsLong: 0.5}),
		spec("hs_sel_back_extension", "HS バック・エクステンション（セレクト）", 2.5, stimulus{training.Erector: 1.0, training.Glute: 0.5, training.Hamstring: 0.4}),
		spec("hs_sel_abdominal_crunch", "HS アブドミナル・クランチ（セレクト）", 2.5, stimulus{training.Abs: 1.0, training.Oblique: 0.3}),
		spec("hs_sel_seated_leg_press", "HS シーテッド・レッグプレス（セレクト）", 5.0, stimulus{training.Quad: 1.0, training.Glute: 0.5, training.Adductor: 0.3}),
		spec("hs_sel_leg_extension", "HS レッグエクステンション（セレクト）", 2.5, stimulus{training.Quad: 1.0}),
		spec("hs_sel_leg_curl", "HS レッグカール（セレクト）", 2.5, stimulus{training.Hamstring: 1.0}),
		spec("hs_sel_seated_leg_curl", "HS シーテッド・レッグカール（セレクト）", 2.5, stimulus{training.Hamstring: 1.0}),
		spec("hs_sel_hip_and_glute", "HS ヒップ・アンド・グルート（セレクト）", 2.5, stimulus{training.Glute: 1.0, training.Hamstring: 0.3}),
		spec("hs_sel_hip_abduction", "HS ヒップ・アブダクション（セレクト）", 2.5, stimulus{training.Glute: 1.0}),
		spec("hs_sel_hip_adduction", "HS ヒップ・アダクション（セレクト）", 2.5, stimulus{training.Adductor: 1.0}),
		spec("hs_sel_standing_calf", "HS スタンディング・カーフ（セレクト）", 2.5, stimulus{training.Calf: 1.0}),
		spec("hs_sel_horizontal_calf", "HS ホリゾンタル・カーフ（セレクト）", 2.5, stimulus{training.Calf: 1.0}),
	}
}
