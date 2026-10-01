import { describe, expect, it } from 'vitest';
import { planRecord, planUndo } from './useRecordOrchestrator';

const plan = { exercise_id: 'bench' };
const values = { weight: 100, reps: 8, rir: 2 };
const date = '2026-09-19';

describe('planRecord', () => {
  it('新しいセットは POST 1件だけ', () => {
    const got = planRecord({ plan, recorded: undefined, values, date, newId: () => 'w-new' });

    expect(got.queue).toHaveLength(1);
    expect(got.queue[0]).toEqual({
      path: '/api/set-logs',
      body: {
        logs: [{ id: 'w-new', date, exercise_id: 'bench', weight_kg: 100, reps: 8, rir: 2 }],
      },
    });
  });

  // 同じIDで内容を変えるとサーバーが衝突として弾く契約なので、
  // 先に消してから入れ直す。順序が逆になると、入れた直後に消える。
  it('修正は DELETE を先に積んでから POST する', () => {
    const recorded = { id: 'w-old', weight_kg: 90, reps: 6, rir: 1 };
    const got = planRecord({ plan, recorded, values, date, newId: () => 'w-new' });

    expect(got.queue.map((q) => q.method ?? 'POST')).toEqual(['DELETE', 'POST']);
    expect(got.queue[0]?.path).toBe('/api/set-logs/w-old');
  });

  // 新しいIDにすると、並び順が id 順（＝作った時刻順）なので、直した
  // セットだけが末尾に飛ぶ。1セット目を直したら3セット目になって出てくる。
  it('修正では同じIDを使い回す', () => {
    const recorded = { id: 'w-old', weight_kg: 90, reps: 6, rir: 1 };
    const got = planRecord({ plan, recorded, values, date, newId: () => 'w-new' });

    const body = got.queue[1]?.body as { logs: { id: string }[] };
    expect(body.logs[0]?.id).toBe('w-old');
    expect(got.local.set.id).toBe('w-old');
    expect(got.local.replacing).toBe('w-old');
  });

  // 過去のセットを直しただけでタイマーが走ると、いま休んでいる時間が
  // 上書きされる。
  it('休憩を始めるのは新しく積んだときだけ', () => {
    expect(planRecord({ plan, recorded: undefined, values, date, newId: () => 'a' }).startRest).toBe(true);
    const recorded = { id: 'w-old', weight_kg: 90, reps: 6, rir: 1 };
    expect(planRecord({ plan, recorded, values, date, newId: () => 'a' }).startRest).toBe(false);
  });

  it('IDに使えない文字を経路に入れない', () => {
    const recorded = { id: 'w/old?x', weight_kg: 90, reps: 6, rir: 1 };
    const got = planRecord({ plan, recorded, values, date, newId: () => 'a' });
    expect(got.queue[0]?.path).toBe('/api/set-logs/w%2Fold%3Fx');
  });
});

describe('planUndo', () => {
  it('DELETE を1件積み、手元からも消す', () => {
    const recorded = { id: 'w-old', weight_kg: 90, reps: 6, rir: 1 };
    const got = planUndo({ plan, recorded });

    expect(got.queue).toEqual([{ path: '/api/set-logs/w-old', method: 'DELETE' }]);
    expect(got.forget).toEqual({ exerciseId: 'bench', id: 'w-old' });
  });
});
