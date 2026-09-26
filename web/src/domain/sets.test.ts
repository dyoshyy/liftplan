import { describe, expect, it } from 'vitest';
import { formatLast, formatSets, parseSetInput } from './sets';

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

describe('parseSetInput', () => {
  it('数として読めれば値を返す', () => {
    expect(parseSetInput('102.5', '8', '2')).toEqual({ ok: true, values: { weight: 102.5, reps: 8, rir: 2 } });
  });

  // RIR 0 は「限界まで」で、正しい記録。0 を弾くと限界まで追い込んだ日が
  // 記録できない。
  it('RIR は 0 を通す', () => {
    expect(parseSetInput('100', '5', '0')).toEqual({ ok: true, values: { weight: 100, reps: 5, rir: 0 } });
  });

  it.each([
    { weight: '', reps: '8', rir: '2' },
    { weight: '100', reps: '', rir: '2' },
    { weight: '100', reps: '8', rir: '' },
  ])('空欄があれば止める（$weight / $reps / $rir）', ({ weight, reps, rir }) => {
    expect(parseSetInput(weight, reps, rir)).toEqual({ ok: false, warning: '重量・レップ・RIR を入れてください' });
  });

  // 0kg や 0レップが通ると、推定1RM が 0 に引きずられる。
  it.each([
    { weight: '0', reps: '8', rir: '2' },
    { weight: '100', reps: '0', rir: '2' },
    { weight: '100', reps: '8', rir: '-1' },
  ])('0 以下は止める（$weight / $reps / $rir）', ({ weight, reps, rir }) => {
    expect(parseSetInput(weight, reps, rir)).toEqual({
      ok: false,
      warning: '0 より大きい重量とレップを入れてください',
    });
  });
});
