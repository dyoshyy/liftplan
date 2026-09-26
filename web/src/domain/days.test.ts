import { describe, expect, it } from 'vitest';
import type { Day, RecordedSet } from '../api/types';
import { applyDayChange, patchDays } from './days';

const set = (id: string, weight_kg = 100, reps = 8, rir = 2): RecordedSet => ({ id, weight_kg, reps, rir });

const days: Day[] = [
  {
    date: '2026-09-25',
    total_sets: 3,
    exercises: [
      { exercise_id: 'squat', name: 'スクワット', sets: [set('a'), set('b')] },
      { exercise_id: 'leg_curl', name: 'レッグカール', sets: [set('c', 40)] },
    ],
  },
  {
    date: '2026-09-23',
    total_sets: 1,
    exercises: [{ exercise_id: 'bench', name: 'ベンチプレス', sets: [set('d', 80)] }],
  },
];

const put = (date: string, exerciseId: string, s: RecordedSet, name = '') =>
  ({ kind: 'put', date, exerciseId, name, set: s }) as const;
const remove = (date: string, exerciseId: string, id: string) =>
  ({ kind: 'remove', date, exerciseId, id }) as const;

describe('patchDays: put', () => {
  // 修正は同じIDで入れ直す。末尾に足すと、1セット目を直したら3セット目に
  // なって出てくる。
  it('同じIDのセットはその場で置き換える', () => {
    const got = patchDays(days, put('2026-09-25', 'squat', set('a', 105, 6)));
    expect(got[0]?.exercises[0]?.sets).toEqual([set('a', 105, 6), set('b')]);
    expect(got[0]?.total_sets).toBe(3);
  });

  it('新しいセットは種目の末尾に足し、セット数を数え直す', () => {
    const got = patchDays(days, put('2026-09-25', 'squat', set('e')));
    expect(got[0]?.exercises[0]?.sets.map((s) => s.id)).toEqual(['a', 'b', 'e']);
    expect(got[0]?.total_sets).toBe(4);
  });

  it('その日に無い種目は末尾に足す', () => {
    const got = patchDays(days, put('2026-09-25', 'calf_raise', set('e', 60), 'カーフレイズ'));
    expect(got[0]?.exercises.map((e) => e.exercise_id)).toEqual(['squat', 'leg_curl', 'calf_raise']);
    expect(got[0]?.exercises[2]?.name).toBe('カーフレイズ');
    expect(got[0]?.total_sets).toBe(4);
  });

  // 今日の1セット目は、履歴にまだその日が無い。足さないと、今日記録した
  // 分がアプリを開き直すまで履歴に出ない。
  it('無い日は新しい順を保って差し込む', () => {
    const newest = patchDays(days, put('2026-09-26', 'bench', set('e', 80), 'ベンチプレス'));
    expect(newest.map((d) => d.date)).toEqual(['2026-09-26', '2026-09-25', '2026-09-23']);
    expect(newest[0]).toEqual({
      date: '2026-09-26',
      total_sets: 1,
      exercises: [{ exercise_id: 'bench', name: 'ベンチプレス', sets: [set('e', 80)] }],
    });

    const between = patchDays(days, put('2026-09-24', 'bench', set('e', 80)));
    expect(between.map((d) => d.date)).toEqual(['2026-09-25', '2026-09-24', '2026-09-23']);
  });

  // 手元の状態は React が前の値と比べて描き直す。書き換えると、描き
  // 直されないか、取ってある過去の月まで一緒に変わる。
  it('元の配列を書き換えない', () => {
    const before = structuredClone(days);
    patchDays(days, put('2026-09-25', 'squat', set('a', 105)));
    patchDays(days, remove('2026-09-25', 'squat', 'a'));
    expect(days).toEqual(before);
  });
});

describe('patchDays: remove', () => {
  it('セットを消し、セット数を数え直す', () => {
    const got = patchDays(days, remove('2026-09-25', 'squat', 'a'));
    expect(got[0]?.exercises[0]?.sets.map((s) => s.id)).toEqual(['b']);
    expect(got[0]?.total_sets).toBe(2);
  });

  // 空の種目が残ると「レッグカール」と名前だけの行が出る。
  it('空になった種目は消す', () => {
    const got = patchDays(days, remove('2026-09-25', 'leg_curl', 'c'));
    expect(got[0]?.exercises.map((e) => e.exercise_id)).toEqual(['squat']);
  });

  // 空の日が残ると「0種目 0セット」のカードが出て、月の回数にも数えられる。
  it('空になった日は消す', () => {
    const got = patchDays(days, remove('2026-09-23', 'bench', 'd'));
    expect(got.map((d) => d.date)).toEqual(['2026-09-25']);
  });

  // 待ち行列の再送や二度押しで、もう無いものを消しに来ることがある。
  it('無いものを消しても何も変わらない', () => {
    expect(patchDays(days, remove('2026-09-25', 'squat', 'zzz'))).toEqual(days);
    expect(patchDays(days, remove('2026-01-01', 'squat', 'a'))).toEqual(days);
  });
});

describe('applyDayChange', () => {
  const doneToday = new Map([
    ['squat', [set('a'), set('b')]],
    ['leg_curl', [set('c', 40)]],
  ]);
  const state = { days, doneToday };

  // 今日の記録が履歴に出ないのは、今日の画面が doneToday だけを進めて
  // days を進めていなかったから（開き直すまで履歴に出なかった）。
  it('今日の変更は days と doneToday の両方に当てる', () => {
    const got = applyDayChange(state, put('2026-09-25', 'squat', set('e')), '2026-09-25');
    expect(got.days[0]?.exercises[0]?.sets.map((s) => s.id)).toEqual(['a', 'b', 'e']);
    expect(got.doneToday.get('squat')?.map((s) => s.id)).toEqual(['a', 'b', 'e']);
  });

  // 今日の最後の1セットを消したら、その種目は「0セット済み」に戻る。
  // 古い一覧が残ると、今日の画面のマスが埋まったままになる。
  it('今日の種目が空になれば doneToday も空にする', () => {
    const got = applyDayChange(state, remove('2026-09-25', 'leg_curl', 'c'), '2026-09-25');
    expect(got.doneToday.get('leg_curl')).toEqual([]);
  });

  // 過去の日を直して今日のマスが動くと、今日やったことが変わって見える。
  it('過去の日の変更は doneToday に触らない', () => {
    const got = applyDayChange(state, put('2026-09-23', 'squat', set('e')), '2026-09-26');
    expect(got.doneToday).toBe(doneToday);
    expect(got.days.find((d) => d.date === '2026-09-23')?.total_sets).toBe(2);
  });
});
