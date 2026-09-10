// Package training はトレーニングの進行を司るドメイン層の共有カーネルである。
//
// この層は Onion Architecture の最内周にあたり、外側（Application、
// Infrastructure、Presentation）を一切知らない。標準ライブラリ以外への
// 依存を持たないため、DB も HTTP も立てずに全機能をテストできる。
//
// # パッケージの分け方
//
// ディレクトリは集約で割る。フォルダが集約、その中のファイル名が型と役割。
//
//	training/           どの集約にも属さない値（日付・単位・筋区分・推定1RM）
//	training/exercise/  種目マスタ
//	training/setlog/    実績ログとその集まり
//	training/condition/ 日次コンディション
//	training/program/   ユーザー設定（頻度・週目標・伸ばしたい種目）
//	training/planning/  ドメインサービス。集約を材料に「今日の計画」を導く
//	training/seed/      初期データ
//
// 集約のパッケージは、エンティティ・値オブジェクト・リポジトリの
// インターフェース（Reader / Writer）・その集約に固有のセンチネルを持つ。
// 集約をまたいで判断するものは全て planning に置く。
//
// # なぜドメインサービスを1つのパッケージにまとめるか
//
// 非公開フィールドしか持たない型を、それを組み立てるサービスと同じ
// パッケージに置くため。PlannedSet は SessionPlanner からしか作れず、
// 「提示セットは計画器から出たものだけ」が言語レベルで保証されている。
// 別パッケージに置くと公開コンストラクタが要り、誰でも任意の値で
// 組み立てられるようになる。PlannedSession・StimulusCoverage・
// SlotTemplate も同じ。
//
// ドメインサービスを役割ごとにさらに割ると、この保証が消える。
// 見通しはファイル名で足りる。サービスのファイル名は役割を表す名詞で
// 終わる。
//
//	session_planner.go        その日のセッションを導出する
//	accessory_selector.go     残差から補助種目を選ぶ
//	condition_analyzer.go     体重と睡眠から補正を導く
//	one_rep_max_estimator.go  記録から推定1RMを求める
//	slot_catalog.go           週の何本目かに強度帯を割り当てる
//
// # 依存の向き
//
// 集約のパッケージは共有カーネル（このパッケージ）と、値として持つ
// 他集約の識別子だけを知る。planning は全てを知る。逆向きは無い。
//
//	planning  → exercise, setlog, condition, program, training
//	program   → exercise, training
//	setlog    → exercise, training
//	condition → training
//	exercise  → training
//	training  → （なし）
package training
