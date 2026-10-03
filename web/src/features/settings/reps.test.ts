import { describe, expect, it } from 'vitest';
import { REP_OPTIONS, repsPath, repsRows } from './reps';
import type { Program } from '../../api/types';

const program = (reps: Program['declared_reps']): Program => ({
  per_week: 3,
  exercises_per_session: 4,
  sets_per_exercise: 3,
  selected_exercises: ['bench', 'pull_up', 'squat'],
  declared_exercises: ['bench', 'pull_up'],
  focus_exercise: 'bench',
  splits: [],
  declared_reps: reps,
});

describe('REP_OPTIONS', () => {
  // サーバーの範囲（program.minAxisReps〜maxAxisReps）と同じ。ずれると
  // 選べるのに 400 が返る値か、選べない正当な値ができる。
  it('1〜15 を並べる', () => {
    expect(REP_OPTIONS).toEqual([1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15]);
  });
});

describe('repsPath', () => {
  // 種目 ID は利用者が足した種目だとサーバーが振るが、念のため符号化する。
  it('ID を符号化して口を組む', () => {
    expect(repsPath('pull_up')).toBe('/api/program/declared/pull_up/reps');
    expect(repsPath('a/b')).toBe('/api/program/declared/a%2Fb/reps');
  });
});

describe('repsRows', () => {
  it('宣言の順に並べる', () => {
    const rows = repsRows(program({ bench: { heavy: 3, light: 6 }, pull_up: { heavy: 8, light: 12 } }));
    expect(rows).toEqual([
      { id: 'bench', reps: { heavy: 3, light: 6 } },
      { id: 'pull_up', reps: { heavy: 8, light: 12 } },
    ]);
  });

  // 宣言を足した直後、取り直すまで値が無いことがある。既定を手元で作らず、
  // 行を出さない（既定値はサーバーだけが持つ）。
  it('値が無い宣言は出さない', () => {
    expect(repsRows(program({ bench: { heavy: 3, light: 6 } }))).toEqual([
      { id: 'bench', reps: { heavy: 3, light: 6 } },
    ]);
  });
});
