import { describe, expect, it } from 'vitest';
import { planHistoryDelete, planHistoryEdit } from './useHistoryEditor';

const target = {
  date: '2026-08-12',
  exerciseId: 'squat',
  name: 'スクワット',
  index: 0,
  set: { id: 'w-old', weight_kg: 100, reps: 8, rir: 2 },
};

describe('planHistoryEdit', () => {
  // 今日の日付で入れ直すと、8月の記録が今日へ移る。推定1RMの日付も
  // 週の充足も動き、8月からは消える。
  it('記録の日付のまま入れ直す', () => {
    const got = planHistoryEdit(target, { weight: 105, reps: 6, rir: 1 });
    const body = got.queue[1]?.body as { logs: { id: string; date: string }[] };
    expect(body.logs[0]).toMatchObject({ id: 'w-old', date: '2026-08-12', weight_kg: 105, reps: 6, rir: 1 });
    expect(got.change).toEqual({
      kind: 'put',
      date: '2026-08-12',
      exerciseId: 'squat',
      name: 'スクワット',
      set: { id: 'w-old', weight_kg: 105, reps: 6, rir: 1 },
    });
  });

  // 同じIDで内容を変えるとサーバーが衝突として弾く。
  it('消してから入れ直す', () => {
    const got = planHistoryEdit(target, { weight: 105, reps: 6, rir: 1 });
    expect(got.queue.map((q) => q.method ?? 'POST')).toEqual(['DELETE', 'POST']);
    expect(got.queue[0]?.path).toBe('/api/set-logs/w-old');
  });
});

describe('planHistoryDelete', () => {
  it('そのセットだけを消す', () => {
    const got = planHistoryDelete(target);
    expect(got.queue).toEqual([{ path: '/api/set-logs/w-old', method: 'DELETE' }]);
    expect(got.change).toEqual({ kind: 'remove', date: '2026-08-12', exerciseId: 'squat', id: 'w-old' });
  });
});
