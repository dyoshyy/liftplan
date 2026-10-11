import { describe, expect, it } from 'vitest';
import { weightSummary } from './weightTrend';

describe('weightSummary', () => {
  it('記録が無ければ出さない', () => {
    expect(weightSummary([])).toBeNull();
  });

  it('1件だけなら現在値だけで、増減は出さない（比べる相手が無い）', () => {
    expect(weightSummary([{ date: '2026-08-10', kg: 70.8 }])).toEqual({ currentKg: 70.8, changeKg: null });
  });

  it('現在値は最後の点、増減は最初の点との差', () => {
    expect(
      weightSummary([
        { date: '2026-08-01', kg: 72 },
        { date: '2026-08-10', kg: 70.8 },
        { date: '2026-08-12', kg: 70.4 },
      ]),
    ).toEqual({ currentKg: 70.4, changeKg: -1.6 });
  });

  it('増減は浮動小数の誤差を丸める（-1.5999… を -1.6 に）', () => {
    expect(
      weightSummary([
        { date: 'a', kg: 70.1 },
        { date: 'b', kg: 70.3 },
      ])?.changeKg,
    ).toBe(0.2);
  });
});
