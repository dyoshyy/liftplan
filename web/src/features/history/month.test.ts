import { describe, expect, it } from 'vitest';
import type { Day } from '../../api/types';
import {
  canGoNext,
  daysIn,
  monthLabel,
  monthOf,
  monthRange,
  pickMonthDays,
  planMonthLoad,
  shiftMonth,
  summarize,
} from './month';

const day = (date: string, total_sets: number): Day => ({ date, exercises: [], total_sets });

describe('monthOf', () => {
  it('日付から年月を取る', () => {
    expect(monthOf('2026-09-26')).toBe('2026-09');
  });
});

describe('monthRange', () => {
  // 月末を31日で決め打ちすると、2月や30日の月でサーバーが日付として
  // 読めずに 400 を返し、その月の記録が一件も出なくなる。
  it.each([
    { month: '2026-09', from: '2026-09-01', to: '2026-09-30' },
    { month: '2026-02', from: '2026-02-01', to: '2026-02-28' },
    { month: '2028-02', from: '2028-02-01', to: '2028-02-29' },
    { month: '2026-12', from: '2026-12-01', to: '2026-12-31' },
  ])('$month は $from〜$to', ({ month, from, to }) => {
    expect(monthRange(month)).toEqual({ from, to });
  });
});

describe('shiftMonth', () => {
  it.each([
    { month: '2026-09', delta: -1, want: '2026-08' },
    { month: '2026-01', delta: -1, want: '2025-12' },
    { month: '2026-12', delta: 1, want: '2027-01' },
  ])('$month に $delta で $want', ({ month, delta, want }) => {
    expect(shiftMonth(month, delta)).toBe(want);
  });
});

describe('monthLabel', () => {
  it('年と月を出す', () => {
    expect(monthLabel('2026-09')).toBe('2026年9月');
  });
});

describe('canGoNext', () => {
  // 未来の月には記録が無い。押せると、空の月が延々と続く。
  it('今月からは進めない', () => {
    expect(canGoNext('2026-09', '2026-09-26')).toBe(false);
  });

  it('先月からは進める', () => {
    expect(canGoNext('2026-08', '2026-09-26')).toBe(true);
  });

  it('年をまたいでも比べられる', () => {
    expect(canGoNext('2025-12', '2026-01-03')).toBe(true);
  });
});

describe('daysIn', () => {
  it('その月の日だけを、並びを変えずに返す', () => {
    const days = [day('2026-10-01', 1), day('2026-09-30', 2), day('2026-09-01', 3), day('2026-08-31', 4)];
    expect(daysIn(days, '2026-09').map((d) => d.date)).toEqual(['2026-09-30', '2026-09-01']);
  });
});

describe('summarize', () => {
  it('回数とセット数を数える', () => {
    expect(summarize([day('2026-09-02', 13), day('2026-09-01', 10)])).toEqual({ sessions: 2, sets: 23 });
  });

  it('空なら 0', () => {
    expect(summarize([])).toEqual({ sessions: 0, sets: 0 });
  });
});

describe('planMonthLoad', () => {
  const today = '2026-09-26';

  // 今月は起動時に読んだ直近の記録で描ける。取りに行くと、ジムで履歴を
  // 開くたびに往復が1本増え、圏外では今月の記録まで見えなくなる。
  it('今月は手元の記録を使う', () => {
    expect(planMonthLoad('2026-09', today, new Set(['2026-09']))).toEqual({ kind: 'recent' });
    expect(planMonthLoad('2026-09', today, new Set())).toEqual({ kind: 'recent' });
  });

  it('取ってある月は取り直さない', () => {
    expect(planMonthLoad('2026-08', today, new Set(['2026-08']))).toEqual({ kind: 'cached' });
  });

  it('無い月はその月の範囲で取りに行く', () => {
    expect(planMonthLoad('2026-02', today, new Set())).toEqual({
      kind: 'fetch',
      path: '/api/set-logs?from=2026-02-01&to=2026-02-28',
    });
  });
});

describe('pickMonthDays', () => {
  const today = '2026-09-26';
  const recent = [day('2026-09-25', 13), day('2026-08-31', 12)];

  // 今月を取ってきた結果で描くと、起動後に記録した分（手元にしか無い）が
  // 履歴から消える。
  it('今月は手元の記録から切り出す', () => {
    const fetched = new Map([['2026-09', [day('2026-09-01', 1)]]]);
    expect(pickMonthDays('2026-09', today, recent, fetched)?.map((d) => d.date)).toEqual(['2026-09-25']);
  });

  // 手元の直近56日に先月の一部が入っていても、それで描かない。月の頭の
  // ほうが欠けたまま「この月は3回」と出る。
  it('過去の月は取ってきた結果を使う', () => {
    const fetched = new Map([['2026-08', [day('2026-08-31', 12), day('2026-08-03', 13)]]]);
    expect(pickMonthDays('2026-08', today, recent, fetched)?.map((d) => d.date)).toEqual([
      '2026-08-31',
      '2026-08-03',
    ]);
  });

  // 読み込み中を空として出すと、記録がある月に「記録はありません」が一瞬出る。
  it('まだ取ってきていなければ null', () => {
    expect(pickMonthDays('2026-08', today, recent, new Map())).toBeNull();
  });
});
