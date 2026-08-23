// Package training はトレーニングの進行を司るドメイン層である。
//
// この層は Onion Architecture の最内周にあたり、外側（Application、
// Infrastructure、Presentation）を一切知らない。標準ライブラリ以外への
// 依存を持たないため、DB も HTTP も立てずに全機能をテストできる。
//
// # ファイルの分け方
//
// ドメインサービスのファイル名は、役割を表す名詞で終わる。
//
//	session_planner.go        その日のセッションを導出する
//	accessory_selector.go     残差から補助種目を選ぶ
//	deload_policy.go          停滞を判定してデロードを提案する
//	condition_analyzer.go     体重と睡眠から補正を導く
//	one_rep_max_estimator.go  記録から推定1RMを求める
//	slot_catalog.go           週の何本目かに強度帯を割り当てる
//
// それ以外は型（エンティティ・値オブジェクト・リポジトリのインターフェース）で、
// ファイル名は型の名前に揃える。
//
// ディレクトリでは分けない。分けるとパッケージが割れ、いま非公開で守れて
// いるものを公開せざるを得なくなる。たとえば PlannedSet は非公開フィールド
// しか持たないので SessionPlanner からしか作れないが、別パッケージにすると
// 公開コンストラクタが要り、誰でも任意の値で組み立てられるようになる。
// 見通しを付けたいだけならファイル名で足りる。
package training
