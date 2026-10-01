import { describe, expect, it } from 'vitest';
import { leftovers } from './leftovers';
import type { PlannedSet, RecordedSet } from '../../api/types';

const plan = (id: string): PlannedSet => ({
  exercise_id: id,
  weight_kg: null,
  sets: 3,
  target_rir: 2,
  target_reps: 10,
});

const set = (id: string): RecordedSet => ({ id, weight_kg: 80, reps: 8, rir: 2 });

const done = (m: Record<string, number>) =>
  new Map(
    Object.entries(m).map(([id, n]) => [id, Array.from({ length: n }, (_, i) => set(`${id}-${i}`))]),
  );

describe('leftovers', () => {
  it('予定に無いものだけを返す', () => {
    const got = leftovers([[plan('bench')], [], [plan('curl')]], done({ bench: 3, dips: 2 }));
    expect(got).toEqual([
      { exercise_id: 'dips', weight_kg: null, sets: 2, target_rir: 0, target_reps: 0, finished_only: true },
    ]);
  });

  // レーンを渡し忘れると、その種目が「今日やったもの」に二重で出る。
  it('バリエーションレーンも予定として数える', () => {
    const lanes = [[plan('bench')], [plan('larsen_press')], [plan('curl')]];
    expect(leftovers(lanes, done({ larsen_press: 4 }))).toEqual([]);
  });

  it('1セットも記録していないものは出さない', () => {
    expect(leftovers([[]], done({ dips: 0 }))).toEqual([]);
  });

  it('種目IDの昇順で並べる', () => {
    const got = leftovers([[]], done({ zz: 1, aa: 1, mm: 1 }));
    expect(got.map((p) => p.exercise_id)).toEqual(['aa', 'mm', 'zz']);
  });
});
