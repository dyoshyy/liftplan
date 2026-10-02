package seed

import (
	"github.com/dyoshyy/liftplan/internal/domain/training"
	"github.com/dyoshyy/liftplan/internal/domain/training/exercise"
)

// hammerStrengthSpecs は Hammer Strength のマシンのプリセット。
//
// **本人が通うジム（エニタイムフィットネス 品川中延店・戸越公園店）にある
// マシンだけ。**公式カタログ（Life Fitness の Plate-Loaded・MTS・Select、
// 計80台あまり）を全部入れると多すぎ、使わないニッチなマシンが並ぶため、
// 店舗の「マシンラインナップ」ページ（2026-10-03 時点）に載っていて、機種名が
// Hammer Strength のカタログと一致するものに絞った。
//
//   - 品川中延店 https://www.anytimefitness.co.jp/shinagawanakanobu/facility/
//     フリーウェイトエリアの、アイソラテラル系9台・Tバー・ロー・ラテラルレイズ・
//     プリーチャーカール（カタログの Seated Biceps）・リニアレッグプレス・
//     ハックスクワット・Vスクワット（同 Reverse V-Squat）・グルートドライブ
//   - 戸越公園店 https://www.anytimefitness.co.jp/togoshikoen/facility/
//     Hammer Strength と判別できるマシンは載っていない（Matrix など別ブランドで、
//     機種名もカタログと一致しない）
//
// 入れていないマシン：品川中延店のマシンエリア（チェストプレス・ペックフライ・
// リアデルト・ラットプルダウン・シーテッドロー・アシストチンニング・ディップス
// など）は、ページにブランドが書かれていないので判別できず、入れていない。
// 筋区分に寄与を付けられないマシン（ティビアドルシフレクション、バーチカル・
// ニープラス）も入れていない。
//
// **1台1種目。**同じ動作でも、マシンが違えば重量の刻みも感触も違い、
// 推定1RMは種目ごとに持つ。名前は「HS ＋ 機種名 ＋（プレート）」で、汎用の種目
// （レッグプレスなど）と見分けがつくようにする。
//
// **寄与（効き方）と刻みは仮置き。**アイソラテラル系の9種目は、種目の管理の設計書
// （docs/specs/2026-09-26-custom-exercises-design.md 付録）の初期案どおり。残りは
// 同じ動作の汎用の種目の寄与に揃えた推定で、実機の感触で直すものとして、
// 種目の管理から名前・効き方・刻みを直せる。刻みは、脚を押す・しゃがむマシンと
// 臀筋のマシンを 5kg、ほかを 2.5kg にした。
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
		spec("hs_pl_iso_decline_chest_press", "HS アイソラテラル・デクライン・チェスト・プレス（プレート）", 2.5, stimulus{training.ChestLower: 1.0, training.TricepsLateral: 0.5}),
		spec("hs_pl_iso_shoulder_press", "HS アイソラテラル・ショルダー・プレス（プレート）", 2.5, stimulus{training.FrontDelt: 1.0, training.SideDelt: 0.5, training.TricepsLateral: 0.5, training.TrapUpper: 0.3}),
		spec("hs_pl_iso_row", "HS アイソラテラル・ロー（プレート）", 2.5, stimulus{training.TrapMid: 1.0, training.Lat: 0.5, training.Biceps: 0.5, training.RearDelt: 0.5, training.TrapUpper: 0.3}),
		spec("hs_pl_iso_high_row", "HS アイソラテラル・ハイ・ロー（プレート）", 2.5, stimulus{training.Lat: 1.0, training.TrapMid: 0.5, training.RearDelt: 0.5, training.Biceps: 0.5, training.TrapUpper: 0.3}),
		spec("hs_pl_iso_low_row", "HS アイソラテラル・ロー・ロー（プレート）", 2.5, stimulus{training.Lat: 1.0, training.TrapMid: 0.5, training.Biceps: 0.5, training.TrapUpper: 0.3}),
		spec("hs_pl_iso_dy_row", "HS アイソラテラル・DY・ロー（プレート）", 2.5, stimulus{training.Lat: 1.0, training.TrapMid: 0.5, training.Biceps: 0.5, training.TrapUpper: 0.3}),
		spec("hs_pl_iso_wide_pulldown", "HS アイソラテラル・ワイド・プルダウン（プレート）", 2.5, stimulus{training.Lat: 1.0, training.TrapMid: 0.5, training.Biceps: 0.5}),
		spec("hs_pl_iso_front_pulldown", "HS アイソラテラル・フロント・プルダウン（プレート）", 2.5, stimulus{training.Lat: 1.0, training.Biceps: 0.5}),

		// --- プレート式 ---
		spec("hs_pl_t_bar_row", "HS Tバー・ロー（プレート）", 2.5, stimulus{training.TrapMid: 1.0, training.Lat: 0.8, training.RearDelt: 0.4, training.Biceps: 0.3, training.Erector: 0.3, training.TrapUpper: 0.3}),
		spec("hs_pl_lateral_raise", "HS ラテラルレイズ（プレート）", 2.5, stimulus{training.SideDelt: 1.0, training.TrapUpper: 0.3}),
		spec("hs_pl_seated_biceps", "HS シーテッド・バイセップス（プレート）", 2.5, stimulus{training.Biceps: 1.0, training.Forearm: 0.3}),
		spec("hs_pl_reverse_v_squat", "HS リバースVスクワット（プレート）", 5.0, stimulus{training.Quad: 1.0, training.Glute: 0.7, training.Adductor: 0.3, training.Erector: 0.3}),
		spec("hs_pl_hack_squat", "HS ハックスクワット（プレート）", 5.0, stimulus{training.Quad: 1.0, training.Glute: 0.5, training.Adductor: 0.2}),
		spec("hs_pl_linear_leg_press", "HS リニア・レッグプレス（プレート）", 5.0, stimulus{training.Quad: 1.0, training.Glute: 0.5, training.Adductor: 0.3}),
		spec("hs_pl_glute_drive", "HS グルート・ドライブ（プレート）", 5.0, stimulus{training.Glute: 1.0, training.Hamstring: 0.4}),

		// --- セレクト ---
	}
}
