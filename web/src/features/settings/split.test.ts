import { describe, expect, it } from 'vitest';
import { matchingPresetKey, splitBody } from './split';
import type { Program, SplitPreset } from '../../api/types';

const preset = (key: string, splits: SplitPreset['splits']): SplitPreset => ({
  key,
  name: key,
  splits,
});

const upperLower = preset('upper_lower', [
  { name: '上半身', regions: ['CHEST_MID', 'LAT'] },
  { name: '下半身', regions: ['GLUTE', 'QUAD'] },
]);

const program = (splits: Program['splits']): Program => ({
  per_week: 4,
  exercises_per_session: 4,
  sets_per_exercise: 3,
  weekly_target: {},
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
