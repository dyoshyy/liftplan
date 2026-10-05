import type { TrendPoint } from '../../api/types';

export type TrendLayout = {
  /** 左端を 0、右端を 1 とした横位置。日付の間隔に比例する。 */
  x: number[];
  /** 上端を 0、下端を 1 とした縦位置。 */
  y: number[];
  min: number;
  max: number;
  /** 最高値の点。並んだら新しいほう。 */
  bestIndex: number;
};

const DAY_MS = 86_400_000;

// UTC で読む。ローカルで読むと夏時間のある環境で1日が23時間になる。
const dayNumber = (iso: string) => Math.round(Date.parse(`${iso}T00:00:00Z`) / DAY_MS);

/** layoutTrend は点を 0〜1 の座標に置く。描画の寸法は呼び出し側が掛ける。
 *
 *  points は古い順（サーバーの並び）。 */
export function layoutTrend(points: readonly TrendPoint[]): TrendLayout {
  const kgs = points.map((p) => p.kg);
  const min = Math.min(...kgs);
  const max = Math.max(...kgs);
  const span = max - min;

  const first = dayNumber(points[0]?.date ?? '');
  const last = dayNumber(points[points.length - 1]?.date ?? '');
  const days = last - first;

  const x = points.map((p) => (days > 0 ? (dayNumber(p.date) - first) / days : 0.5));
  const y = points.map((p) => (span > 0 ? (max - p.kg) / span : 0.5));

  let bestIndex = 0;
  kgs.forEach((kg, i) => {
    if (kg >= (kgs[bestIndex] ?? -Infinity)) bestIndex = i;
  });

  return { x, y, min, max, bestIndex };
}

/** nearestIndex は横位置（0〜1）にいちばん近い点。指で押した位置から点を選ぶ。 */
export function nearestIndex(xs: readonly number[], ratio: number): number {
  let best = 0;
  xs.forEach((x, i) => {
    if (Math.abs(x - ratio) < Math.abs((xs[best] ?? 0) - ratio)) best = i;
  });
  return best;
}

/** stepIndex は選んだ点を前後に動かす。端で止まる。 */
export const stepIndex = (i: number, delta: number, n: number): number =>
  Math.min(n - 1, Math.max(0, i + delta));
