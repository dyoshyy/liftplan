import { describe, expect, it } from 'vitest';
import type { Program, SplitPreset } from '../../api/types';
import { draftFrom, planOnboardingRequests, withFrequency, type PutRequest } from './onboarding';

const upperLower: SplitPreset = {
  key: 'upper_lower',
  name: '上下2分割',
  splits: [
    { name: '上半身', regions: ['CHEST_MID', 'LAT'] },
    { name: '下半身', regions: ['GLUTE', 'QUAD'] },
  ],
  min_frequency_per_week: 0,
};

const fiveWay: SplitPreset = {
  key: 'five_way',
  name: '5分割',
  splits: [
    { name: '胸', regions: ['CHEST_MID'] },
    { name: '背中', regions: ['LAT'] },
    { name: '肩', regions: ['SIDE_DELT'] },
    { name: '腕', regions: ['BICEPS'] },
    { name: '脚', regions: ['QUAD'] },
  ],
  min_frequency_per_week: 4,
};

const presets = [upperLower, fiveWay];

// 新規の利用者に入る既定（seed.DefaultProgram）と同じ形。
const base: Program = {
  per_week: 3,
  exercises_per_session: 4,
  sets_per_exercise: 3,
  selected_exercises: ['bench', 'deadlift', 'squat', 'row'],
  declared_exercises: ['bench', 'deadlift', 'squat'],
  focus_exercise: null,
  splits: [],
  declared_reps: {},
};

const paths = (rs: PutRequest[]) => rs.map((r) => r.path);

describe('draftFrom', () => {
  it('全身法は split=null', () => {
    expect(draftFrom(base, presets)).toEqual({
      perWeek: 3,
      split: null,
      exercises: 4,
      sets: 3,
      declared: ['bench', 'deadlift', 'squat'],
    });
  });

  it('保存された分割と一致するプリセットを選んだ状態にする', () => {
    expect(draftFrom({ ...base, splits: upperLower.splits }, presets).split).toBe(upperLower);
  });
});

describe('withFrequency', () => {
  it('下限に足りなくなった分割は全身法に戻す', () => {
    const d = { ...draftFrom(base, presets), perWeek: 5, split: fiveWay };
    expect(withFrequency(d, 3)).toMatchObject({ perWeek: 3, split: null });
  });

  it('足りていれば分割を残す', () => {
    const d = { ...draftFrom(base, presets), split: upperLower };
    expect(withFrequency(d, 2).split).toBe(upperLower);
  });
});

describe('planOnboardingRequests', () => {
  it('全身法・週3 → 5分割・週5 は回数を先に送る（今の週3では5分割を選べない）', () => {
    const d = { ...draftFrom(base, presets), perWeek: 5, split: fiveWay };
    expect(paths(planOnboardingRequests(base, d))).toEqual([
      '/api/program/frequency',
      '/api/program/split',
      '/api/program/volume',
      '/api/program/declared',
      '/api/onboarding/completed',
    ]);
  });

  it('5分割・週5 → 全身法・週2 は分割を先に送る（途中で失敗して選び直した場合）', () => {
    const current = { ...base, per_week: 5, splits: fiveWay.splits };
    const d = { ...draftFrom(current, presets), perWeek: 2, split: null };
    expect(paths(planOnboardingRequests(current, d)).slice(0, 2)).toEqual([
      '/api/program/split',
      '/api/program/frequency',
    ]);
  });

  it('本文は設定画面が送るものと同じ形', () => {
    const d = {
      ...draftFrom(base, presets),
      perWeek: 4,
      exercises: 5,
      sets: 2,
      declared: ['squat', 'bench'],
    };
    const body = (path: string) => planOnboardingRequests(base, d).find((r) => r.path === path)?.body;
    expect(body('/api/program/frequency')).toEqual({ per_week: 4 });
    expect(body('/api/program/split')).toEqual({ splits: [] });
    expect(body('/api/program/volume')).toEqual({ exercises_per_session: 5, sets_per_exercise: 2 });
    // サーバーは昇順に正規化して返す。揃えておかないと設定画面の並びが入れ替わる。
    expect(body('/api/program/declared')).toEqual({ declared_exercises: ['bench', 'squat'] });
  });

  it('最後は必ず完了の記録で、本文を持たない', () => {
    const rs = planOnboardingRequests(base, draftFrom(base, presets));
    expect(rs.at(-1)).toEqual({ path: '/api/onboarding/completed' });
  });
});
