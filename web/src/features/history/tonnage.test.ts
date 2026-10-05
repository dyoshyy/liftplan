import { describe, expect, it } from 'vitest';
import type { Day } from '../../api/types';
import { dayTonnage, formatTonnage } from './tonnage';

const day = (sets: [number, number][]): Day => ({
  date: '2026-09-22',
  total_sets: sets.length,
  exercises: [
    {
      exercise_id: 'bench',
      name: 'ベンチ',
      sets: sets.map(([weight_kg, reps], i) => ({ id: `s${i}`, weight_kg, reps, rir: 2 })),
    },
  ],
});

describe('dayTonnage', () => {
  it('重量×回数を全セット足す', () => {
    expect(
      dayTonnage(
        day([
          [80, 8],
          [82.5, 6],
        ]),
      ),
    ).toBe(80 * 8 + 82.5 * 6);
  });
});

describe('formatTonnage', () => {
  it.each([
    [0, '0kg'],
    [840, '840kg'],
    [999.6, '1,000kg'],
    [12_345, '12.3t'],
  ])('%f → %s', (kg, want) => {
    expect(formatTonnage(kg)).toBe(want);
  });
});
