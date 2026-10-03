import { describe, expect, it } from 'vitest';
import { describePutFailure, exercisesSummary } from './useProgramSettings';
import type { Program } from '../../api/types';

// サーバーが本文に書いた理由（5分割の頻度下限のような、状態コードだけでは
// 本人に伝わらない理由）をそのまま出す。理由が無ければ状態コードだけを出す
// （web/src/dev/simulate.ts の describeFailure と同じ考え方）。
describe('describePutFailure', () => {
  it('サーバーが書いた理由をそのまま出す', () => {
    const message = '5分割は週4回以上が必要です。先に分割を変えてください';
    expect(describePutFailure(400, { error: message })).toBe(message);
  });

  it('理由が空文字なら状態コードだけを出す', () => {
    expect(describePutFailure(400, { error: '' })).toBe('変えられませんでした（400）');
  });

  it('本文が読めなければ状態コードだけを出す', () => {
    expect(describePutFailure(500, null)).toBe('変えられませんでした（500）');
  });
});

const program = (focus: string | null): Program => ({
  per_week: 3,
  exercises_per_session: 4,
  sets_per_exercise: 3,
  selected_exercises: ['bench', 'row', 'squat'],
  declared_exercises: ['bench', 'squat'],
  focus_exercise: focus,
  splits: [],
  declared_reps: {},
});

const nameOf = (id: string) => ({ bench: 'ベンチプレス', squat: 'スクワット' })[id] ?? id;

describe('exercisesSummary', () => {
  // 名前は長さが決まっていないので最後に置く。狭い画面で切れても件数は残る。
  it('使う・伸ばす・重点の順に並べる', () => {
    expect(exercisesSummary(program('bench'), nameOf)).toBe('使う3・伸ばす2・重点 ベンチプレス');
  });

  it('重点が無ければ「重点なし」と出す', () => {
    expect(exercisesSummary(program(null), nameOf)).toBe('使う3・伸ばす2・重点なし');
  });
});
