import { describe, expect, it } from 'vitest';
import {
  addSetTarget,
  planHistoryAdd,
  planHistoryDelete,
  planHistoryEdit,
  type EditTarget,
} from './useHistoryEditor';

const target: EditTarget = {
  kind: 'edit',
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

describe('planHistoryAdd', () => {
  const add = { date: '2026-08-12', exerciseId: 'squat', name: 'スクワット' };
  const values = { weight: 100, reps: 8, rir: 2 };

  // 足すセットには消す相手が無い。DELETE を積むと、採番したばかりの
  // ID を消しに行くだけでなく、planRecord の「直す」経路に入ったことになる。
  it('入れるだけで、消さない', () => {
    const got = planHistoryAdd(add, values, () => 'w-new');
    expect(got.queue.map((q) => q.method ?? 'POST')).toEqual(['POST']);
  });

  // 今日の日付で送ると、8月に足したつもりのセットが今日に入る。
  it('足す日の日付で、採番した ID で送る', () => {
    const got = planHistoryAdd(add, values, () => 'w-new');
    const body = got.queue[0]?.body as { logs: { id: string; date: string }[] };
    expect(body.logs).toEqual([
      { id: 'w-new', date: '2026-08-12', exercise_id: 'squat', weight_kg: 100, reps: 8, rir: 2 },
    ]);
  });

  // 手元に当てるセットの ID が送ったものと違うと、足した直後に直す・消すが
  // 別のセットを指す。
  it('手元にも同じ ID・同じ日で足す', () => {
    const got = planHistoryAdd(add, values, () => 'w-new');
    expect(got.change).toEqual({
      kind: 'put',
      date: '2026-08-12',
      exerciseId: 'squat',
      name: 'スクワット',
      set: { id: 'w-new', weight_kg: 100, reps: 8, rir: 2 },
    });
  });
});

describe('addSetTarget', () => {
  const log = {
    exercise_id: 'squat',
    name: 'スクワット',
    sets: [
      { id: 'a', weight_kg: 100, reps: 8, rir: 2 },
      { id: 'b', weight_kg: 90, reps: 10, rir: 1 },
    ],
  };

  it('次のセット番号は、その種目のセット数', () => {
    expect(addSetTarget('2026-08-12', log).index).toBe(2);
  });

  // 最後のセットで重量を落としていたなら、足すのもその続きのことが多い。
  // 1セット目に戻すと、落とした判断が無かったことになる（今日の記録シートと同じ）。
  it('入力は最後のセットの値で始める', () => {
    expect(addSetTarget('2026-08-12', log).initial).toMatchObject({ weight_kg: 90, reps: 10, rir: 1 });
  });

  it('その日・その種目に足す', () => {
    expect(addSetTarget('2026-08-12', log)).toMatchObject({
      kind: 'add',
      date: '2026-08-12',
      exerciseId: 'squat',
      name: 'スクワット',
    });
  });

  // 一覧の行と同じ。名前が引けない種目でも見出しが空にならない。
  it('名前が無ければ ID を出す', () => {
    expect(addSetTarget('2026-08-12', { ...log, name: '' }).name).toBe('squat');
  });
});
