import type { Day } from '../../api/types';
import { monthRange, type Month } from './month';

export type CalendarCell = {
  date: string;
  /** 日（1〜31）。 */
  day: number;
  /** その日のセット数。記録が無ければ 0。 */
  sets: number;
};

/** monthGrid は月を日曜始まりの週ごとに並べる。月の外は null。 */
export function monthGrid(m: Month, days: readonly Day[]): (CalendarCell | null)[][] {
  const { from, to } = monthRange(m);
  const last = Number(to.slice(8));
  const lead = new Date(`${from}T00:00:00`).getDay();
  const sets = new Map(days.map((d) => [d.date, d.total_sets]));

  const cells: (CalendarCell | null)[] = Array.from({ length: lead }, () => null);
  for (let d = 1; d <= last; d++) {
    const date = `${m}-${String(d).padStart(2, '0')}`;
    cells.push({ date, day: d, sets: sets.get(date) ?? 0 });
  }
  while (cells.length % 7 !== 0) cells.push(null);

  return Array.from({ length: cells.length / 7 }, (_, w) => cells.slice(w * 7, w * 7 + 7));
}
