// Package group は「記録を見せ合う仲間の集まり」を表すドメイン層のパッケージである。
//
// account と同じく training の外側に置く。グループはトレーニングの何かでは
// なく、トレーニングをする人どうしの関係である。
//
// このパッケージが持つ一番大事な判断は「誰が誰の記録を見てよいか」で、
// それは Group.VisibleTo だけが決める。他人の記録を読む口（ActivityReader）は
// VisibleTo の結果（Audience）しか受け取らないので、判断を飛ばして読む
// コードはコンパイルが通らない。既存のリポジトリが「他人の記録は見えない」を
// 契約にしている壁（postgres/cross_user_test.go）に、穴をこの1か所だけ開ける
// （docs/specs/2026-10-11-groups-design.md）。
//
// training と同じく、DB も HTTP も立てずにテストできる。
package group
