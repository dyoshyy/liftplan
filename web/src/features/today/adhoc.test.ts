import { describe, expect, it } from 'vitest';
import type { PlannedSet, RecordedSet } from '../../api/types';
import { adhocCards, belowPlan, plannedIds, withPicked } from './adhoc';

const plan = (id: string): PlannedSet => ({
  exercise_id: id,
  weight_kg: null,
  sets: 3,
  target_rir: 2,
  target_reps: 10,
});

describe('plannedIds', () => {
  // 3レーンの全部を見ないと、バリエーションや補助が選択肢に残る。
  it('3レーンのどれに出ている種目も集める', () => {
    const lanes = [[plan('bench')], [plan('squat')], [plan('dip')]];
    expect([...plannedIds(lanes)].sort()).toEqual(['bench', 'dip', 'squat']);
  });

  it('予定が空なら空', () => {
    expect(plannedIds([[], [], []]).size).toBe(0);
  });
});

describe('withPicked', () => {
  it('選んだ順に足す', () => {
    expect(withPicked(['dip'], 'squat')).toEqual(['dip', 'squat']);
  });

  // 同じ種目を2回選ぶと、同じカードが2枚出て key も衝突する。
  it('選び済みなら足さない', () => {
    expect(withPicked(['dip', 'squat'], 'dip')).toEqual(['dip', 'squat']);
  });
});

describe('adhocCards', () => {
  it('選んだ種目を、重量未定・セット数0の計画として返す', () => {
    const got = adhocCards(['dip'], [[]]);
    expect(got).toEqual([
      { exercise_id: 'dip', weight_kg: null, sets: 0, target_rir: 2, target_reps: 0, adhoc: true },
    ]);
  });

  // 選んだあとで予定に入った（計画を取り直した）種目は、予定のカードに
  // 任せる。両方に出すと同じ種目が2枚並ぶ。
  it('予定に入った種目は返さない', () => {
    expect(adhocCards(['dip', 'squat'], [[plan('dip')]]).map((p) => p.exercise_id)).toEqual(['squat']);
  });

  it('選んだ順を保つ', () => {
    expect(adhocCards(['squat', 'dip'], [[]]).map((p) => p.exercise_id)).toEqual(['squat', 'dip']);
  });
});

describe('belowPlan', () => {
  const rec = (id: string): RecordedSet => ({ id, weight_kg: 60, reps: 10, rir: 2 });
  const done = (m: Record<string, number>) =>
    new Map(Object.entries(m).map(([id, n]) => [id, Array.from({ length: n }, (_, i) => rec(`${id}-${i}`))]));
  const ids = (cards: readonly { exercise_id: string }[]) => cards.map((p) => p.exercise_id);

  // 選んだ種目を記録すると doneToday に入る。選んだ分と記録の分で2枚に
  // ならない。
  it('選んで記録した種目は1枚だけ', () => {
    expect(ids(belowPlan([[plan('bench')], [], []], ['dip'], done({ dip: 2 })))).toEqual(['dip']);
  });

  // 選んだ状態は画面のメモリにしか無く、開き直すと消える。記録の数だけの
  // 枠で出すと、続きを記録する空き枠が無くなる。予定に無い種目をやったのは
  // 本人が選んだからなので、記録から選んだ種目に戻す。
  it('選んだ状態が消えても、予定に無い記録は選んだ種目として出る', () => {
    expect(belowPlan([[plan('bench')], [], []], [], done({ dip: 2 }))).toEqual([
      { exercise_id: 'dip', weight_kg: null, sets: 0, target_rir: 2, target_reps: 0, adhoc: true },
    ]);
  });

  it('予定に出ている種目は出さない', () => {
    expect(belowPlan([[plan('bench')], [], []], ['bench'], done({ bench: 3 }))).toEqual([]);
  });

  // 3レーンの全部を予定として数えないと、バリエーションや補助の記録が
  // 選んだ種目として二重に出る。
  it('どのレーンの記録も予定として数える', () => {
    const lanes = [[plan('bench')], [plan('larsen_press')], [plan('curl')]];
    expect(belowPlan(lanes, [], done({ bench: 1, larsen_press: 4, curl: 2 }))).toEqual([]);
  });

  it('選んだだけで記録の無い種目も出る', () => {
    expect(ids(belowPlan([[]], ['dip'], done({})))).toEqual(['dip']);
  });

  it('1セットも記録していない種目は、記録からは出さない', () => {
    expect(belowPlan([[]], [], done({ dip: 0 }))).toEqual([]);
  });

  // 選んだ順を先に、記録から戻したものはその後ろに種目IDの昇順で並べる。
  // 記録のたびに並びが入れ替わらないように。
  it('選んだ順、その後ろに記録から戻した種目を昇順で', () => {
    expect(ids(belowPlan([[]], ['zz'], done({ mm: 1, aa: 1, zz: 1 })))).toEqual(['zz', 'aa', 'mm']);
  });
});
