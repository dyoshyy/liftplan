// Package planning はドメインサービスを集めたパッケージ。
//
// 集約（exercise・setlog・condition・program）を材料に「今日どう動くか」を
// 導く。ここに置く型は全て、それを組み立てるサービスと同じパッケージに
// 居ることで不変条件を守っている。PlannedSet は SessionPlanner からしか
// 作れず、StimulusCoverage は残差計算からしか出てこない。
//
// ファイル名は役割を表す名詞で終わる（_planner / _selector / _policy /
// _analyzer / _estimator / _catalog）。それ以外は型で、ファイル名は型に揃える。
package planning
