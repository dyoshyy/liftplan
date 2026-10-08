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

  // 選んだ種目を記録すると doneToday に入る。予定に無いので leftovers が
  // 「今日やったもの」にも出し、同じ種目が2枚並ぶ。選んだ種目は予定として
  // 数えて、あちらから外す。
  it('選んだ種目は「今日やったもの」に二重に出ない', () => {
    const got = belowPlan([[plan('bench')], [], []], ['dip'], done({ dip: 2 }));
    expect(got.adhoc.map((p) => p.exercise_id)).toEqual(['dip']);
    expect(got.done).toEqual([]);
  });

  // 画面を開き直すと、選んだ状態は消える。やった事実は消えない。
  it('選んでいないがやったものは「今日やったもの」に出る', () => {
    const got = belowPlan([[plan('bench')], [], []], [], done({ dip: 2 }));
    expect(got.adhoc).toEqual([]);
    expect(got.done.map((p) => p.exercise_id)).toEqual(['dip']);
  });

  it('予定に出ている種目はどちらにも出さない', () => {
    const got = belowPlan([[plan('bench')], [], []], ['bench'], done({ bench: 3 }));
    expect(got.adhoc).toEqual([]);
    expect(got.done).toEqual([]);
  });

  it('選んだだけで記録の無い種目は、選んだ種目として出る', () => {
    const got = belowPlan([[]], ['dip'], done({}));
    expect(got.adhoc.map((p) => p.exercise_id)).toEqual(['dip']);
    expect(got.done).toEqual([]);
  });
});
