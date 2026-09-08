import { describe, expect, it } from 'vitest';
import { formatLast, formatSets } from './sets';

describe('formatSets', () => {
  // 1セット目の重量で代表させると、落とした重量も上げた重量も履歴から消える。
  it.each([
    {
      name: '同じ重量はまとめる',
      sets: [
        { weight_kg: 100, reps: 8 },
        { weight_kg: 100, reps: 8 },
        { weight_kg: 100, reps: 7 },
      ],
      want: '100kg × 8, 8, 7',
    },
    {
      name: '重量の変わり目で区切る',
      sets: [
        { weight_kg: 100, reps: 8 },
        { weight_kg: 90, reps: 8 },
      ],
      want: '100kg × 8　/　90kg × 8',
    },
    {
      name: '同じ重量に戻ったら別の区切りになる',
      sets: [
        { weight_kg: 100, reps: 8 },
        { weight_kg: 90, reps: 8 },
        { weight_kg: 100, reps: 5 },
      ],
      want: '100kg × 8　/　90kg × 8　/　100kg × 5',
    },
    { name: '記録が無ければ空', sets: [], want: '' },
  ])('$name', ({ sets, want }) => {
    expect(formatSets(sets)).toBe(want);
  });
});

describe('formatLast', () => {
  it('セットごとの重量が無ければ代表の重量で埋める', () => {
    expect(formatLast({ weight_kg: 80, reps: [8, 8], days_ago: 3 })).toBe('80kg × 8, 8');
  });

  it('セットごとの重量があればそちらを使う', () => {
    expect(formatLast({ weight_kg: 80, weights: [80, 70], reps: [8, 10], days_ago: 3 })).toBe(
      '80kg × 8　/　70kg × 10',
    );
  });
});
