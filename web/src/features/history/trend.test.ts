import { describe, expect, it } from 'vitest';
import { layoutTrend, nearestIndex, stepIndex } from './trend';

const p = (date: string, kg: number) => ({ date, kg });

describe('layoutTrend', () => {
  // 点を等間隔に置くと、2週間空いたあとの1点が隣の点と同じ近さに見え、
  // 「ずっと続けていた」ように読める。
  it('x は日付の間隔に比例する', () => {
    const l = layoutTrend([p('2026-09-01', 100), p('2026-09-02', 101), p('2026-09-11', 102)]);
    expect(l.x).toEqual([0, 0.1, 1]);
  });

  it('y は大きいほど上（0 が上端）', () => {
    const l = layoutTrend([p('2026-09-01', 100), p('2026-09-08', 110), p('2026-09-15', 105)]);
    expect(l.y).toEqual([1, 0, 0.5]);
  });

  // 同じ値だけだと幅が 0 になり、割って NaN になる。
  it('値が全部同じでも NaN を出さない', () => {
    const l = layoutTrend([p('2026-09-01', 100), p('2026-09-08', 100)]);
    expect(l.y.every(Number.isFinite)).toBe(true);
  });

  it('点が1つなら中央に置く', () => {
    const l = layoutTrend([p('2026-09-01', 100)]);
    expect(l.x).toEqual([0.5]);
    expect(l.y).toEqual([0.5]);
  });

  it('最高値の位置と値の範囲を返す', () => {
    const l = layoutTrend([p('2026-09-01', 100), p('2026-09-08', 110), p('2026-09-15', 105)]);
    expect(l).toMatchObject({ min: 100, max: 110, bestIndex: 1 });
  });

  // 同値の最高は直近を採る。「いま最高」と読めるほうが励みになる。
  it('最高値が並んだら新しいほう', () => {
    const l = layoutTrend([p('2026-09-01', 110), p('2026-09-08', 100), p('2026-09-15', 110)]);
    expect(l.bestIndex).toBe(2);
  });
});

describe('nearestIndex', () => {
  const xs = [0, 0.1, 1];
  it.each([
    [0, 0],
    [0.04, 0],
    [0.06, 1],
    [0.4, 1],
    [0.6, 2],
    [1, 2],
  ])('x=%f は %d 番目', (ratio, want) => {
    expect(nearestIndex(xs, ratio)).toBe(want);
  });
});

describe('stepIndex', () => {
  it('端で止まる', () => {
    expect(stepIndex(0, -1, 3)).toBe(0);
    expect(stepIndex(2, 1, 3)).toBe(2);
    expect(stepIndex(1, 1, 3)).toBe(2);
  });
});
