import { describe, expect, it } from 'vitest';
import type { Day } from '../../api/types';
import { monthGrid } from './calendar';

const day = (date: string, sets: [number, number][]): Day => ({
  date,
  total_sets: sets.length,
  exercises: [
    {
      exercise_id: 'bench',
      name: 'ベンチ',
      sets: sets.map(([weight_kg, reps], i) => ({ id: `${date}-${i}`, weight_kg, reps, rir: 2 })),
    },
  ],
});

describe('monthGrid', () => {
  // 2026-09-01 は火曜。日曜始まりなので先頭に空きが2つ入る。
  it('月初の曜日に合わせて先頭を空ける', () => {
    const g = monthGrid('2026-09', []);
    expect(g[0]?.slice(0, 3)).toEqual([null, null, expect.objectContaining({ date: '2026-09-01' })]);
  });

  it('末日までを週ごとに並べ、最後の週の余りは空ける', () => {
    const g = monthGrid('2026-09', []);
    expect(g.every((w) => w.length === 7)).toBe(true);
    const cells = g.flat().filter((c) => c !== null);
    expect(cells).toHaveLength(30);
    expect(cells.at(-1)?.date).toBe('2026-09-30');
  });

  // 2月は28日で、決め打ちの31日だと存在しない日付ができる。
  it('2月は28日（うるう年は29日）', () => {
    expect(monthGrid('2026-02', []).flat().filter(Boolean)).toHaveLength(28);
    expect(monthGrid('2028-02', []).flat().filter(Boolean)).toHaveLength(29);
  });

  it('記録のある日にセット数を入れる', () => {
    const g = monthGrid('2026-09', [
      day('2026-09-22', [
        [80, 8],
        [80, 8],
      ]),
    ]);
    const cell = g.flat().find((c) => c?.date === '2026-09-22');
    expect(cell).toMatchObject({ day: 22, sets: 2 });
    expect(g.flat().find((c) => c?.date === '2026-09-23')).toMatchObject({ sets: 0 });
  });
});
