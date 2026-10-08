// Package planning はドメインサービスを集めたパッケージ。
//
// 集約（exercise・setlog・condition・program）を材料に「今日どう動くか」を
// 導く。ここに置く型は全て、それを組み立てるサービスと同じパッケージに
// 居ることで不変条件を守っている。PlannedSet は SessionPlanner からしか
// 作れず、StimulusCoverage は CoverageBetween や Plus からしか出てこない。
//
// サービスのファイル名は役割を表す名詞で終わる（_planner / _allocator /
// _analyzer / _estimator / _projector）。レーン（*_lane.go・axis_rotation.go・
// lane_prescription.go）は計画器が使う非公開の判断。それ以外は型か関数で、
// ファイル名はそれに揃える。
package planning
