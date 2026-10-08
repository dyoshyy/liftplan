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
// 組み立てられるようになる。PlannedSession と StimulusCoverage も同じ。
//
// ドメインサービスを役割ごとにさらに割ると、この保証が消える。
// 見通しはファイル名で足りる。サービスのファイル名は役割を表す名詞で
// 終わる。
//
//	_planner    その日のセッションを導出する
//	_allocator  残差から補助種目を割り振る
//	_analyzer   睡眠から目標RIRの補正を導く
//	_estimator  記録から推定1RMを求める
//	_projector  先の回の分割・軸・バリエーションを見込む
//
// サービスでないファイルもある。レーン（*_lane.go・axis_rotation.go・
// lane_prescription.go）は SessionPlanner が使う非公開の判断で、役割の名詞では
// なく「どのレーンの何か」で名付ける。残りは型か関数で、名前をそれに揃える。
//
// ファイル名を並べずに接尾辞で書いているのは、一覧はファイルの増減の
// たびに古くなるため。実際、消えたファイルがここに載り続けていた。
// いま在るファイルは ls が答える。ここに残すのは、ls では分からない
// 「その名前が何の役割を指すか」だけ。
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
