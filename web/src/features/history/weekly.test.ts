import { describe, expect, it } from 'vitest';
import { weeklyTotal } from './weekly';

const v = (done: number, target: number) => ({ region: 'LAT', done_sets: done, target_sets: target });

describe('weeklyTotal', () => {
  it('区分をまたいで合計する', () => {
    expect(weeklyTotal([v(6, 12), v(3, 8)])).toEqual({ done: 9, target: 20, pct: 45 });
  });

  // 目標が 0 のときに割ると NaN が出て、棒の幅が NaN% になる。
  it('目標が 0 でも割らない', () => {
    expect(weeklyTotal([v(0, 0)]).pct).toBe(0);
  });

  it('超過しても 100 で止める', () => {
    expect(weeklyTotal([v(20, 10)]).pct).toBe(100);
  });

  it('空なら 0', () => {
    expect(weeklyTotal([])).toEqual({ done: 0, target: 0, pct: 0 });
  });
});
