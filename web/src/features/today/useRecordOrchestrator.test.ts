import { describe, expect, it } from 'vitest';
import {
  planRecord,
  planUndo,
  rollbackRecord,
  rollbackUndo,
  saveFailureMessage,
} from './useRecordOrchestrator';

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

// 記録を押した瞬間に手元の記録を進める（楽観的更新）。端末に保存できなかった
// ときは、画面を「実際に保存されている状態」へ戻す。戻さないと、画面には
// 記録済みなのに何も保存されていない、という食い違いが残る。
describe('rollbackRecord', () => {
  it('新しいセットは、手元の記録から取り除く', () => {
    const planned = planRecord({ plan, recorded: undefined, values, date, newId: () => 'w-new' });

    expect(rollbackRecord(planned, undefined)).toEqual({
      kind: 'forget',
      exerciseId: 'bench',
      id: 'w-new',
    });
  });

  // 修正が保存できなかったときに取り除くと、元からあった記録まで画面から
  // 消える。保存されているのは修正前の値なので、それに戻す。
  it('修正は、修正前の値に戻す', () => {
    const previous = { id: 'w-old', weight_kg: 90, reps: 6, rir: 1 };
    const got = planRecord({ plan, recorded: previous, values, date, newId: () => 'unused' });

    expect(rollbackRecord(got, previous)).toEqual({
      kind: 'restore',
      exerciseId: 'bench',
      set: previous,
    });
  });
});

describe('rollbackUndo', () => {
  // 取り消しが保存できなかったとき、記録はサーバーに残ったまま。画面からだけ
  // 消えた状態にしない。
  it('取り消したセットを、手元の記録へ戻す', () => {
    const removed = { id: 'w-old', weight_kg: 90, reps: 6, rir: 1 };
    const got = planUndo({ plan, recorded: removed });

    expect(rollbackUndo(got, removed)).toEqual({
      kind: 'restore',
      exerciseId: 'bench',
      set: removed,
    });
  });
});

// 戻したことを、何を記録しようとしたかで伝える。「保存できませんでした」
// だけでは、どの記録を入れ直せばよいか分からない。
describe('saveFailureMessage', () => {
  const set = { id: 'w-1', weight_kg: 100, reps: 5, rir: 2 };

  it('記録は、種目名・重量・レップで言う', () => {
    expect(saveFailureMessage('record', 'ベンチプレス', set)).toBe('ベンチプレス 100kg × 5');
  });

  it('取り消しは、取り消そうとした記録を言う', () => {
    expect(saveFailureMessage('undo', 'ベンチプレス', set)).toBe('ベンチプレス 100kg × 5 の取り消し');
  });
});

// 操作のIDは、シートを開いた1回で決まる（draftId）。同じ操作を2回通しても
// 同じIDで同じ内容になれば、サーバーも手元の記録も、2回目を無害に吸収する。
describe('planRecord の冪等性', () => {
  it('同じ操作IDなら、2回計画しても同じ計画になる', () => {
    const first = planRecord({ plan, recorded: undefined, values, date, newId: () => 'draft-1' });
    const second = planRecord({ plan, recorded: undefined, values, date, newId: () => 'draft-1' });

    expect(second).toEqual(first);
  });
});
