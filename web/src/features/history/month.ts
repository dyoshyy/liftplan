import type { Day } from '../../api/types';
import { patchDays, type DayChange } from '../../domain/days';
import { dayTonnage } from './tonnage';

/** Month は `YYYY-MM`。文字列のまま比べると時系列の順になる。 */
export type Month = string;

/** monthOf は `YYYY-MM-DD` の年月。 */
export const monthOf = (date: string): Month => date.slice(0, 7);

const parts = (m: Month) => {
  const [y, mo] = m.split('-').map(Number);
  return { y: y ?? 0, mo: mo ?? 1 };
};

const fmt = (y: number, mo: number) => `${y}-${String(mo).padStart(2, '0')}`;

/** monthRange はその月の初日と末日。サーバーに渡す期間になる。
 *
 *  末日は「翌月の0日」で求める。31日で決め打ちすると、2月や30日の月が
 *  日付として読めずに 400 で返り、その月が丸ごと空に見える。 */
export function monthRange(m: Month): { from: string; to: string } {
  const { y, mo } = parts(m);
  const last = new Date(y, mo, 0).getDate();
  return { from: `${m}-01`, to: `${m}-${String(last).padStart(2, '0')}` };
}

/** shiftMonth は月を前後に動かす。 */
export function shiftMonth(m: Month, delta: number): Month {
  const { y, mo } = parts(m);
  const d = new Date(y, mo - 1 + delta, 1);
  return fmt(d.getFullYear(), d.getMonth() + 1);
}

/** monthLabel は「2026年9月」。 */
export function monthLabel(m: Month): string {
  const { y, mo } = parts(m);
  return `${y}年${mo}月`;
}

/** canGoNext は次の月へ進めるか。未来の月に記録は無いので、今月で止める。 */
export const canGoNext = (m: Month, today: string): boolean => m < monthOf(today);

/** daysIn はその月の日だけを返す。並び（新しい順）は変えない。 */
export const daysIn = (days: readonly Day[], m: Month): Day[] => days.filter((d) => monthOf(d.date) === m);

export type MonthSummary = { sessions: number; sets: number; tonnage: number };

/** summarize は月の頭に出す「何回・何セット・総挙上量」。 */
export function summarize(days: readonly Day[]): MonthSummary {
  return {
    sessions: days.length,
    sets: days.reduce((a, d) => a + d.total_sets, 0),
    tonnage: days.reduce((a, d) => a + dayTonnage(d), 0),
  };
}

export type MonthLoad = { kind: 'recent' } | { kind: 'cached' } | { kind: 'fetch'; path: string };

/** planMonthLoad はその月の記録をどこから持ってくるかを決める。
 *
 *  **今月は取りに行かない。**起動時に読む直近56日の記録（`useLiftplan`）に
 *  今月は必ず収まっている。取りに行くと、ジムで履歴を開くたびに往復が
 *  1本増え、圏外では今月の記録まで見えなくなる。
 *
 *  取ってきた月は取り直さない。過去の記録は、自分で直さない限り動かない。 */
export function planMonthLoad(m: Month, today: string, cached: ReadonlySet<Month>): MonthLoad {
  if (m === monthOf(today)) return { kind: 'recent' };
  if (cached.has(m)) return { kind: 'cached' };
  const { from, to } = monthRange(m);
  return { kind: 'fetch', path: `/api/set-logs?from=${from}&to=${to}` };
}

/** pickMonthDays はその月に描く日を返す。まだ取ってきていなければ null。
 *
 *  今月は手元の記録から切り出す。記録すると手元の記録が先に進むので、
 *  取ってきた結果で描くと、起動後に記録した分が履歴から消える。
 *
 *  過去の月は手元に一部が入っていても使わない。直近56日の窓は月の途中で
 *  切れるので、月の頭が欠けたまま回数が出る。 */
export function pickMonthDays(
  m: Month,
  today: string,
  recent: readonly Day[],
  fetched: ReadonlyMap<Month, Day[]>,
): Day[] | null {
  if (m === monthOf(today)) return daysIn(recent, m);
  return fetched.get(m) ?? null;
}

/** patchFetched は取ってある月に変更を当てる。
 *
 *  取ってある月は取り直さないので、当てないと開き直すまで直す前の値が出る。
 *  **まだ取っていない月は作らない。**「1日だけの月」を作ると取ってきた
 *  ことになり、その月の残りの日が出なくなる。 */
export function patchFetched(
  fetched: ReadonlyMap<Month, Day[]>,
  change: DayChange,
): ReadonlyMap<Month, Day[]> {
  const m = monthOf(change.date);
  const days = fetched.get(m);
  if (!days) return fetched;
  return new Map(fetched).set(m, patchDays(days, change));
}
