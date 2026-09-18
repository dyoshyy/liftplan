import { describe, expect, it } from 'vitest';
import { defaultsForSet } from './defaults';

const plan = { weight_kg: 100, target_rir: 2 };
const last = { weight_kg: 95, weights: [95, 95], reps: [8, 7], days_ago: 3 };

describe('defaultsForSet', () => {
  // 1セット目は、今日まだ何も記録が無いので従来どおり。
  it('今日まだ記録が無ければ、処方と前回から決める', () => {
    expect(defaultsForSet({ plan, last, index: 0, doneToday: [], recorded: undefined })).toEqual({
      weight: '100',
      reps: '8',
      rir: '2',
    });
  });

  // ここが今回の変更。前回のセッションではなく、今日の直前のセットに合わせる。
  it('今日すでに記録があれば、直前のセットに合わせる', () => {
    const done = [{ id: 'a', weight_kg: 102.5, reps: 6, rir: 1 }];
    expect(defaultsForSet({ plan, last, index: 1, doneToday: done, recorded: undefined })).toEqual({
      weight: '102.5',
      reps: '6',
      rir: '1',
    });
  });

  // 3セット目で重量を落としたら、4セット目は落とした側から始まってほしい。
  // 1セット目に戻ると、下げた判断が毎回無かったことになる。
  it('直前のセットであって、1セット目ではない', () => {
    const done = [
      { id: 'a', weight_kg: 100, reps: 8, rir: 2 },
      { id: 'b', weight_kg: 100, reps: 7, rir: 1 },
      { id: 'c', weight_kg: 90, reps: 8, rir: 2 },
    ];
    expect(defaultsForSet({ plan, last, index: 3, doneToday: done, recorded: undefined })).toEqual({
      weight: '90',
      reps: '8',
      rir: '2',
    });
  });

  // 直している最中は、そのセット自身の値が出ないと直せない。
  it('記録済みのセットを開いたら、そのセットの値を出す', () => {
    const done = [
      { id: 'a', weight_kg: 100, reps: 8, rir: 2 },
      { id: 'b', weight_kg: 90, reps: 5, rir: 0 },
    ];
    const recorded = done[0];
    expect(defaultsForSet({ plan, last, index: 0, doneToday: done, recorded })).toEqual({
      weight: '100',
      reps: '8',
      rir: '2',
    });
  });

  // 処方が null（＝自分で決める）で、今日の記録も前回も無い場合。
  it('手がかりが何も無ければ重量は空にする', () => {
    const got = defaultsForSet({
      plan: { weight_kg: null, target_rir: 2 },
      last: undefined,
      index: 0,
      doneToday: [],
      recorded: undefined,
    });
    expect(got.weight).toBe('');
    expect(got.reps).toBe('8');
  });
});
