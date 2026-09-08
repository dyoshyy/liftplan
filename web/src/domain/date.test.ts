import { describe, expect, it } from 'vitest';
import { addDays, label, today } from './date';

describe('today', () => {
  // toISOString は UTC に直すので、日本時間の朝9時より前は前日になる。
  // 朝練の記録が前日に付く。
  it('端末の日付を返す（UTC に直さない）', () => {
    expect(today(new Date(2026, 8, 7, 6, 0, 0))).toBe('2026-09-07');
  });

  it('月と日を2桁に揃える', () => {
    expect(today(new Date(2026, 0, 3))).toBe('2026-01-03');
  });
});

describe('addDays', () => {
  it.each([
    { iso: '2026-09-07', days: -56, want: '2026-07-13' },
    { iso: '2026-03-01', days: -1, want: '2026-02-28' },
    { iso: '2026-12-31', days: 1, want: '2027-01-01' },
  ])('$iso に $days 日で $want', ({ iso, days, want }) => {
    expect(addDays(iso, days)).toBe(want);
  });
});

describe('label', () => {
  it('曜日を付ける', () => {
    expect(label('2026-09-07')).toBe('9/7 (月)');
  });
});
