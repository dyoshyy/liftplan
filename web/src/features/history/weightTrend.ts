import type { TrendPoint } from '../../api/types';

/** WeightSummary は体重の推移の見出しに出す数字。 */
export type WeightSummary = {
  currentKg: number;
  /** changeKg は期間の最初の点との差。比べる相手が無い（1件だけ）なら null。 */
  changeKg: number | null;
};

/**
 * weightSummary は体重の推移の見出しの数字。記録が無ければ null。
 *
 * 1件だけのときに「0.0」と出すと、変わっていないように読める。比べる相手が
 * まだ無いだけなので、増減は出さない（推定1RMの推移と同じ扱い）。
 */
export function weightSummary(points: readonly TrendPoint[]): WeightSummary | null {
  const first = points[0];
  const last = points[points.length - 1];
  if (!first || !last) return null;
  return {
    currentKg: last.kg,
    changeKg: points.length >= 2 ? Math.round((last.kg - first.kg) * 10) / 10 : null,
  };
}
