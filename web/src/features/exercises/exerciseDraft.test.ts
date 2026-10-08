import { describe, expect, it } from 'vitest';
import type { Exercise } from '../../api/types';
import {
  afterDelete,
  aliveExercises,
  chipMark,
  deleteBlockedReason,
  cycleRegion,
  hideBlockedReason,
  nextDeleteStep,
  draftBody,
  draftOf,
  draftProblem,
  emptyDraft,
  nextSelected,
  setContribution,
  stimulusSummary,
  type ExerciseDraft,
} from './exerciseDraft';

const ex = (id: string, extra: Partial<Exercise> = {}): Exercise => ({
  id,
  name: id,
  increment_kg: 2.5,
  stimulus: { LAT: 1 },
  ...extra,
});

const withStimulus = (stimulus: ExerciseDraft['stimulus'], name = 'アイソラテラル・ロー'): ExerciseDraft => ({
  ...emptyDraft(),
  name,
  stimulus,
});

describe('emptyDraft', () => {
  it('名前と寄与が空、刻みは初期値を持つ', () => {
    const d = emptyDraft();
    expect(d.name).toBe('');
    expect(d.stimulus).toEqual({});
    expect(d.incrementKg).toBeGreaterThan(0);
  });
});

describe('draftOf', () => {
  it('既存の種目から下書きを作る。寄与はそのまま', () => {
    const e = ex('u-1', { name: 'ロー', stimulus: { LAT: 1, BICEPS: 0.5 }, increment_kg: 5 });
    expect(draftOf(e)).toEqual({ name: 'ロー', stimulus: { LAT: 1, BICEPS: 0.5 }, incrementKg: 5 });
  });

  it('返した下書きの寄与を書き換えても元の種目に影響しない', () => {
    const e = ex('u-1', { stimulus: { LAT: 1 } });
    const d = draftOf(e);
    d.stimulus.LAT = 0.5;
    expect(e.stimulus.LAT).toBe(1);
  });
});

describe('cycleRegion', () => {
  // 無し → 1.0 → 0.5 → 無し と回る。
  it('無し → 1.0 → 0.5 → 無し と回る', () => {
    const a = cycleRegion(emptyDraft(), 'LAT');
    expect(a.stimulus).toEqual({ LAT: 1.0 });
    const b = cycleRegion(a, 'LAT');
    expect(b.stimulus).toEqual({ LAT: 0.5 });
    const c = cycleRegion(b, 'LAT');
    expect(c.stimulus).toEqual({});
  });

  it('1.0・0.5以外の値（0.7など）の区分を押したら1.0にする', () => {
    const d = withStimulus({ LAT: 0.7 });
    expect(cycleRegion(d, 'LAT').stimulus).toEqual({ LAT: 1.0 });
  });

  it('元の下書きを書き換えない', () => {
    const d = emptyDraft();
    cycleRegion(d, 'LAT');
    expect(d.stimulus).toEqual({});
  });
});

describe('setContribution', () => {
  it('数値をそのまま入れる（丸めない）', () => {
    const d = setContribution(emptyDraft(), 'LAT', 0.73);
    expect(d.stimulus).toEqual({ LAT: 0.73 });
  });

  it('範囲外の値もそのまま入れる（検証は draftProblem）', () => {
    const d = setContribution(emptyDraft(), 'LAT', 1.5);
    expect(d.stimulus).toEqual({ LAT: 1.5 });
  });

  it('元の下書きを書き換えない', () => {
    const d = emptyDraft();
    setContribution(d, 'LAT', 1);
    expect(d.stimulus).toEqual({});
  });
});

describe('draftProblem', () => {
  it('名前と寄与1.0の区分があれば送れる', () => {
    expect(draftProblem(withStimulus({ QUAD: 1, GLUTE: 0.5 }))).toBeNull();
  });

  it('名前が空なら止める（前後の空白だけも空）', () => {
    expect(draftProblem(withStimulus({ LAT: 1 }, '   '))).toMatch(/名前/);
  });

  it('名前は40文字まで', () => {
    expect(draftProblem(withStimulus({ LAT: 1 }, 'あ'.repeat(40)))).toBeNull();
    expect(draftProblem(withStimulus({ LAT: 1 }, 'あ'.repeat(41)))).toMatch(/40/);
  });

  // rune数（Unicodeのコードポイント数）で数える。サロゲートペアの絵文字は
  // .length（UTF-16単位）だと2に数えられ、40個で80になる。それでも
  // rune数は40なので通る（サーバーの utf8.RuneCountInString と同じ数え方）。
  it('絵文字は rune 数で数える（.length ではない）', () => {
    expect(draftProblem(withStimulus({ LAT: 1 }, '💪'.repeat(40)))).toBeNull();
  });

  it('寄与1.0の区分が無ければ止める', () => {
    expect(draftProblem(withStimulus({ LAT: 0.5 }))).toMatch(/寄与1\.0の区分/);
  });

  it('区分は合わせて8つまで', () => {
    const regions = [
      'LAT',
      'TRAP_MID',
      'TRAP_UPPER',
      'ERECTOR',
      'BICEPS',
      'FOREARM',
      'REAR_DELT',
      'SIDE_DELT',
      'ABS',
    ];
    const stimulus = Object.fromEntries(regions.map((r, i) => [r, i === 0 ? 1 : 0.5]));
    expect(draftProblem(withStimulus(stimulus))).toMatch(/8/);
    delete stimulus.ABS;
    expect(draftProblem(withStimulus(stimulus))).toBeNull();
  });

  it('寄与は0.1未満または1.0超で止める', () => {
    expect(draftProblem(withStimulus({ LAT: 1, BICEPS: 0.05 }))).toMatch(/0\.1〜1\.0/);
    expect(draftProblem(withStimulus({ LAT: 1, BICEPS: 1.5 }))).toMatch(/0\.1〜1\.0/);
  });

  it('刻みは0より大きく50kg以下', () => {
    expect(draftProblem({ ...withStimulus({ LAT: 1 }), incrementKg: 0 })).toMatch(/刻み/);
    expect(draftProblem({ ...withStimulus({ LAT: 1 }), incrementKg: 51 })).toMatch(/刻み/);
  });
});

describe('draftBody', () => {
  it('名前の前後の空白を落とし、区分を並べて本文を作る', () => {
    const body = draftBody({
      name: '  アイソラテラル・ロー ',
      stimulus: { TRAP_MID: 0.5, LAT: 1, BICEPS: 0.5 },
      incrementKg: 2.5,
    });
    expect(body).toEqual({
      name: 'アイソラテラル・ロー',
      stimulus: { BICEPS: 0.5, LAT: 1, TRAP_MID: 0.5 },
      increment_kg: 2.5,
    });
    expect(Object.keys(body.stimulus)).toEqual(['BICEPS', 'LAT', 'TRAP_MID']);
  });
});

describe('aliveExercises', () => {
  it('消した種目を落とす', () => {
    const got = aliveExercises([ex('bench'), ex('u-1', { deleted: true }), ex('u-2')]);
    expect(got.map((e) => e.id)).toEqual(['bench', 'u-2']);
  });
});

describe('nextSelected', () => {
  it('使わない種目を押すと使う種目に入る（昇順を保つ）', () => {
    expect(nextSelected(['squat'], ['squat'], 'bench')).toEqual(['bench', 'squat']);
  });

  it('使う種目を押すと外れる', () => {
    expect(nextSelected(['bench', 'curl', 'squat'], ['bench', 'squat'], 'curl')).toEqual(['bench', 'squat']);
  });

  // 外すとサーバーが 400 を返す。画面から到達させない。
  it('伸ばしたい種目は外せない（null）', () => {
    expect(nextSelected(['bench', 'squat'], ['bench'], 'bench')).toBeNull();
  });
});

describe('hideBlockedReason', () => {
  it('伸ばしたい種目なら理由を返す', () => {
    expect(hideBlockedReason(['u-1'], 'u-1')).toMatch(/伸ばしたい種目/);
    expect(hideBlockedReason(['bench'], 'u-1')).toBeNull();
  });
});

describe('stimulusSummary', () => {
  it('寄与の大きい順に並べる', () => {
    expect(stimulusSummary({ GLUTE: 0.7, QUAD: 1 })).toBe('大腿四頭筋 1.0・臀筋 0.7');
  });

  it('同点は区分名順', () => {
    // BICEPS < LAT なので、同じ寄与なら BICEPS が先。
    expect(stimulusSummary({ LAT: 0.5, BICEPS: 0.5 })).toBe('上腕二頭筋 0.5・広背筋 0.5');
  });
});

// チップは寄与1.0と0.5で同じ「✓」だった。送れない理由が「寄与1.0の区分が
// 無い」のとき、どのチップが1.0なのかがチップだけでは分からない（#228）。
describe('chipMark', () => {
  it.each([
    { name: '選んでいなければ印は無い', v: undefined, want: '' },
    { name: '寄与1.0は「主」（送るのに1つ以上要る）', v: 1, want: '主' },
    { name: '0.5は「少し」', v: 0.5, want: '少し' },
    // プリセットには 0.7 などもある。1.0 でなければ「主」ではない
    { name: '1.0 未満は全部「少し」', v: 0.7, want: '少し' },
  ])('$name', ({ v, want }) => {
    expect(chipMark(v)).toBe(want);
  });
});

// 種目の削除（2026-10-01 に一度やめ、本人の依頼で戻した）。
describe('deleteBlockedReason', () => {
  // サーバーも 409 で断る。押す前に読めたほうが、次に何をすればいいか分かる。
  it('伸ばしたい種目に入っていれば理由を返す', () => {
    expect(deleteBlockedReason(['u-1'], 'u-1')).toMatch(/伸ばしたい種目/);
    expect(deleteBlockedReason(['bench'], 'u-1')).toBeNull();
  });
});

describe('nextDeleteStep', () => {
  // 消した種目は画面から戻せない（undo が無い）ので、編集・使うの隣の1タップで
  // 即消えると事故になる。同じ種目をもう一度押すまでは消さない。
  it('確認待ちが無いときの1タップは、確認待ちに入るだけでまだ消さない', () => {
    expect(nextDeleteStep(null, 'u-1')).toEqual({ pendingId: 'u-1', act: 'arm' });
  });

  it('確認待ちの種目を続けて押すと確定する', () => {
    expect(nextDeleteStep('u-1', 'u-1')).toEqual({ pendingId: null, act: 'confirm' });
  });

  it('確認待ち中に別の種目を押すと、そちらの確認待ちに切り替わる（前の確認は流れる）', () => {
    expect(nextDeleteStep('u-1', 'u-2')).toEqual({ pendingId: 'u-2', act: 'arm' });
  });
});

describe('afterDelete', () => {
  // サーバーは消した種目を使う種目から外す。手元の「使う種目」が古いままだと、
  // 次に別の種目の「使う」を切り替えたとき消した ID ごと送り、サーバーが
  // 「消した種目は選べない」で 400 を返す。
  it('消した種目を使う種目から外す', () => {
    expect(afterDelete(['bench', 'side_raise', 'squat'], 'side_raise')).toEqual(['bench', 'squat']);
  });

  it('入っていなければそのまま', () => {
    expect(afterDelete(['bench'], 'side_raise')).toEqual(['bench']);
  });

  it('読めていない（null）ならそのまま', () => {
    expect(afterDelete(null, 'side_raise')).toBeNull();
  });
});
