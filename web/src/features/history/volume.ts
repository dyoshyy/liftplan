/**
 * fillPercent は週目標に対する充足を 0〜100 の整数で返す。
 *
 * 棒の幅に入る値なので、**必ず 0〜100 に収める**。超えた量を幅で
 * 表しても読めないし、枠から溢れると「壊れている」に見える。
 * 数字のほうは丸めずに出しているので、超過はそちらで分かる。
 *
 * 目標 0 で割らない。週目標に 0 の区分は入らない設計だが（サーバーが
 * 下限 0.5 で弾く）、割る側に来た瞬間 Infinity になり `width: NaN%` が
 * style に入る。画面は落ちず、棒だけが黙って消える。
 */
export function fillPercent(done: number, target: number): number {
  if (target <= 0) return done > 0 ? 100 : 0;
  return Math.min(100, Math.round((done / target) * 100));
}
