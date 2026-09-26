import { describe, expect, it } from 'vitest';
import { sessionHeading } from './forecast';

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
