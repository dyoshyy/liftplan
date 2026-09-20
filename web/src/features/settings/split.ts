import type { Program, SplitPreset } from '../../api/types';

/**
 * matchingPresetKey は、いまの周期と一致するプリセットのキーを返す。
 * 一致しなければ null。
 *
 * プリセットは選んだ時点で展開して保存するので、保存された設定に
 * 「どれを選んだか」は残らない。画面でどれを押した状態にするかは、
 * 中身を突き合わせて決める。
 *
 * 突き合わせるのは名前と区分の両方。区分だけだと、同じ割り当てに別の
 * 名前を付けたプリセットが増えたときに取り違える。
 */
export function matchingPresetKey(program: Program, presets: readonly SplitPreset[]): string | null {
  for (const p of presets) {
    if (p.splits.length !== program.splits.length) continue;
    const same = p.splits.every((s, i) => {
      const mine = program.splits[i];
      if (mine === undefined || mine.name !== s.name) return false;
      if (mine.regions.length !== s.regions.length) return false;
      return s.regions.every((r, j) => mine.regions[j] === r);
    });
    if (same) return p.key;
  }
  return null;
}

/**
 * splitBody は PUT /api/program/split のボディを組む。
 *
 * プリセットを渡さなければ分割なしに戻る。
 */
export function splitBody(preset: SplitPreset | null): { splits: SplitPreset['splits'] } {
  return { splits: preset ? preset.splits : [] };
}
