import { describe, expect, it } from 'vitest';
import { Unauthorized } from '../api/client';
import {
  buildQuery,
  describeFailure,
  defaultForm,
  formatPct,
  formatWeight,
  outOfRange,
  rate,
  toggle,
  tone,
  withFocusInDeclared,
  type DevSet,
  type DevWeek,
} from './simulate';

describe('buildQuery', () => {
  it('設定をそのまま渡す', () => {
    const q = new URLSearchParams(buildQuery(defaultForm));
    expect(q.get('declared')).toBe('bench,squat,deadlift');
    expect(q.get('focus')).toBe('bench');
    expect(q.get('split')).toBe('upper_lower');
    expect(q.get('frequency')).toBe('4');
    expect(q.get('weeks')).toBe('4');
  });

  // 空文字を送るとサーバーが「そういう名前の分割」を探して 400 になる。
  it('指定していない項目は送らない', () => {
    const q = new URLSearchParams(buildQuery({ ...defaultForm, focus: '', split: '' }));
    expect(q.has('focus')).toBe(false);
    expect(q.has('split')).toBe(false);
  });
});

describe('toggle', () => {
  it('入っていなければ足し、入っていれば外す', () => {
    expect(toggle(['bench'], 'squat')).toEqual(['bench', 'squat']);
    expect(toggle(['bench', 'squat'], 'bench')).toEqual(['squat']);
  });
});

describe('withFocusInDeclared', () => {
  // 宣言から外した種目を重点に残したまま送ると 400 が返る。
  it('重点種目が宣言に無ければ指定を落とす', () => {
    const got = withFocusInDeclared({ ...defaultForm, declared: ['squat'], focus: 'bench' });
    expect(got.focus).toBe('');
  });

  it('宣言に入っていればそのまま', () => {
    expect(withFocusInDeclared(defaultForm).focus).toBe('bench');
  });
});

describe('tone', () => {
  // 許容は 60〜145%。境界がずれると、通し検証が緑なのに画面が赤くなる。
  it.each([
    [0.59, 'low'],
    [0.6, 'ok'],
    [1.45, 'ok'],
    [1.46, 'high'],
  ])('%s は %s', (value, want) => {
    expect(tone(value)).toBe(want);
  });
});

describe('rate', () => {
  it('目標が0なら0。0除算を画面に出さない', () => {
    expect(rate(3, 0)).toBe(0);
    expect(rate(6, 12)).toBe(0.5);
  });
});

describe('outOfRange', () => {
  it('許容から外れた区分だけを返す', () => {
    const week: DevWeek = {
      index: 1,
      regions: [
        { region: 'CHEST_MID', target: 12, done: 12 },
        { region: 'CALF', target: 10, done: 3 },
        { region: 'ABS', target: 10, done: 20 },
      ],
    };
    expect(outOfRange(week).map((r) => r.region)).toEqual(['CALF', 'ABS']);
  });
});

describe('表示', () => {
  const set = (over: Partial<DevSet>): DevSet => ({
    exercise_id: 'bench',
    name: 'ベンチプレス',
    weight_kg: 95,
    sets: 3,
    target_rir: 1,
    pct_of_1rm: 0.88,
    ...over,
  });

  it('重量が未確定なら、本人が決める枠だと分かる形で出す', () => {
    expect(formatWeight(set({ weight_kg: null }))).toBe('自分で決める');
    expect(formatWeight(set({}))).toBe('95kg');
  });

  it('推定1RMに対する比は小数2桁。立っていなければ空', () => {
    expect(formatPct(set({}))).toBe('0.88');
    expect(formatPct(set({ pct_of_1rm: null }))).toBe('');
  });
});

describe('describeFailure', () => {
  // 401 だけは「もう一度押す」で直らない。画面が案内を変える。
  it('認証が切れていたらログインし直す先を出す', () => {
    const got = describeFailure(new Unauthorized());
    expect(got).toContain('ログイン');
    expect(got).not.toBe('unauthorized');
  });

  // サーバーは入力が成り立たない理由を本文に書いて 400 を返す。
  // 握り潰すと、画面には「400 を返した」しか出ない。
  it('サーバーが書いた理由はそのまま出す', () => {
    expect(describeFailure(new Error('クエリ weeks が不正である: zero'))).toBe(
      'クエリ weeks が不正である: zero',
    );
  });

  it('Error でないものも文字列にする', () => {
    expect(describeFailure('落ちた')).toBe('落ちた');
  });
});
