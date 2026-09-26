import { describe, expect, it } from 'vitest';
import type { ForecastSession, PlannedSet } from '../../api/types';
import { forecastRows, isInitiallyOpen, sessionHeading } from './forecast';

const set = (exercise_id: string): PlannedSet => ({
  exercise_id,
  weight_kg: 100,
  sets: 3,
  target_rir: 2,
});

describe('sessionHeading', () => {
  it.each([
    { index: 0, split: null, want: '今日' },
    { index: 1, split: null, want: '次の回' },
    { index: 2, split: null, want: '2回後' },
    { index: 5, split: null, want: '5回後' },
    { index: 6, split: null, want: '6回後' },
  ])('回$index・分割なしは $want', ({ index, split, want }) => {
    expect(sessionHeading(index, split)).toBe(want);
  });

  it.each([
    { index: 0, split: '下半身', want: '今日 ・ 下半身' },
    { index: 1, split: '上半身', want: '次の回 ・ 上半身' },
    { index: 3, split: '肩・腕', want: '3回後 ・ 肩・腕' },
  ])('分割があれば日の名前を添える（回$index）', ({ index, split, want }) => {
    expect(sessionHeading(index, split)).toBe(want);
  });
});

describe('forecastRows', () => {
  it('軸・バリエーション・補助の順に並べる', () => {
    const session: ForecastSession = {
      index: 0,
      split: null,
      main: [set('main-1')],
      variation: [set('variation-1')],
      accessories: [set('accessory-1'), set('accessory-2')],
    };

    expect(forecastRows(session).map((s) => s.exercise_id)).toEqual([
      'main-1',
      'variation-1',
      'accessory-1',
      'accessory-2',
    ]);
  });

  it('空のレーンは飛ばす', () => {
    const session: ForecastSession = {
      index: 0,
      split: null,
      main: [set('main-1')],
      variation: [],
      accessories: [],
    };

    expect(forecastRows(session).map((s) => s.exercise_id)).toEqual(['main-1']);
  });
});

describe('isInitiallyOpen', () => {
  it('今日（回0）だけ開いて始める', () => {
    expect(isInitiallyOpen(0)).toBe(true);
    expect(isInitiallyOpen(1)).toBe(false);
    expect(isInitiallyOpen(2)).toBe(false);
  });
});
