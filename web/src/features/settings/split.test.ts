import { describe, expect, it } from 'vitest';
import { isSplitSelectable, matchingPresetKey, splitBody, splitUnselectableReason } from './split';
import type { Program, SplitPreset } from '../../api/types';

const preset = (
  key: string,
  splits: SplitPreset['splits'],
  minFrequencyPerWeek = 0,
): SplitPreset => ({
  key,
  name: key,
  splits,
  min_frequency_per_week: minFrequencyPerWeek,
});

const upperLower = preset('upper_lower', [
  { name: '上半身', regions: ['CHEST_MID', 'LAT'] },
  { name: '下半身', regions: ['GLUTE', 'QUAD'] },
]);

const program = (splits: Program['splits']): Program => ({
  per_week: 4,
  exercises_per_session: 4,
  sets_per_exercise: 3,
  selected_exercises: [],
  declared_exercises: [],
  focus_exercise: null,
  splits,
});

describe('matchingPresetKey', () => {
  it('中身が一致すればそのキーを返す', () => {
    expect(matchingPresetKey(program(upperLower.splits), [upperLower])).toBe('upper_lower');
  });

  // 分割なしが全身法。画面では「全身法」を押した状態になる。
  it('分割なしはどれとも一致しない', () => {
    expect(matchingPresetKey(program([]), [upperLower])).toBeNull();
  });

  it('区分の数が違えば一致しない', () => {
    const other = program([
      { name: '上半身', regions: ['CHEST_MID'] },
      { name: '下半身', regions: ['GLUTE', 'QUAD'] },
    ]);
    expect(matchingPresetKey(other, [upperLower])).toBeNull();
  });

  // 名前も数も同じで中身だけ違う。長さの検査では捕まらない。
  it('区分の中身が違えば一致しない', () => {
    const other = program([
      { name: '上半身', regions: ['CHEST_MID', 'TRAP_MID'] },
      { name: '下半身', regions: ['GLUTE', 'QUAD'] },
    ]);
    expect(matchingPresetKey(other, [upperLower])).toBeNull();
  });

  // 順序が周期そのもの。並びが違えば別の設定。
  it('並びが違えば一致しない', () => {
    const flipped = program([upperLower.splits[1]!, upperLower.splits[0]!]);
    expect(matchingPresetKey(flipped, [upperLower])).toBeNull();
  });

  it('名前だけ違っても一致しない', () => {
    const renamed = program([
      { name: '押す', regions: ['CHEST_MID', 'LAT'] },
      { name: '下半身', regions: ['GLUTE', 'QUAD'] },
    ]);
    expect(matchingPresetKey(renamed, [upperLower])).toBeNull();
  });
});

describe('splitBody', () => {
  it('プリセットをそのまま送る', () => {
    expect(splitBody(upperLower)).toEqual({ splits: upperLower.splits });
  });

  it('null なら空を送って分割なしに戻す', () => {
    expect(splitBody(null)).toEqual({ splits: [] });
  });
});

// 5分割（PR #185 の計測で週4回未満だと部位ごとの週目標を満たせない）の
// ような下限付きプリセットを、頻度に応じて選べるかどうか判定する。
//
// 下限は preset.min_frequency_per_week から読む。「five_way」という
// キーをここに書かないのは、下限を持つプリセットが増えても画面側を
// 直さずに済ませるため（CLAUDE.md「必要になるまで作らない」）。
describe('isSplitSelectable', () => {
  const fiveWay = preset('five_way', upperLower.splits, 4);

  it('下限が無いプリセットはどの頻度でも選べる', () => {
    expect(isSplitSelectable(upperLower, 1)).toBe(true);
  });

  it('下限未満の頻度では選べない', () => {
    expect(isSplitSelectable(fiveWay, 3)).toBe(false);
  });

  it('下限ちょうどなら選べる', () => {
    expect(isSplitSelectable(fiveWay, 4)).toBe(true);
  });

  it('下限を超えていれば選べる', () => {
    expect(isSplitSelectable(fiveWay, 5)).toBe(true);
  });
});

describe('splitUnselectableReason', () => {
  const fiveWay = preset('five_way', upperLower.splits, 4);

  it('選べるなら理由が無い', () => {
    expect(splitUnselectableReason(fiveWay, 4)).toBeNull();
    expect(splitUnselectableReason(upperLower, 1)).toBeNull();
  });

  it('選べないときは最小頻度を含む理由を返す', () => {
    const reason = splitUnselectableReason(fiveWay, 3);
    expect(reason).not.toBeNull();
    expect(reason).toContain('週4回');
  });
});
