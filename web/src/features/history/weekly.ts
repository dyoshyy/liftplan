import type { Volume } from '../../api/types';

export type WeeklyTotal = { done: number; target: number; pct: number };

/** weeklyTotal は週全体の充足をひとまとめにする。
 *
 *  区分ごとの一覧は「埋まっていない順」に並ぶので、**画面の一番上に
 *  0.0 が何行も出る**。135セット記録した状態でもそうなり、アプリが
 *  動いていないように見える。全体の数字を先に出して、そこを塞ぐ。 */
export function weeklyTotal(volume: readonly Volume[]): WeeklyTotal {
  const done = volume.reduce((a, v) => a + v.done_sets, 0);
  const target = volume.reduce((a, v) => a + v.target_sets, 0);
  return { done, target, pct: target > 0 ? Math.min(100, (done / target) * 100) : 0 };
}
