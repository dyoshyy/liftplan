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
 * isWholeBody は分割なし（全身法）かどうかを見る。
 *
 * matchingPresetKey の null と混ぜない。あちらの null は「どのプリセットにも
 * 一致しない」で、分割そのものは残っている（例: サーバー側でプリセット名が
 * 変わった）。ここで null を全身法と取り違えると、summary には分割名が出て
 * いるのに画面は「全身法」ボタンを選んだ状態で表示してしまう。全身法は
 * 分割が0件のときだけ。
 */
export function isWholeBody(program: Program): boolean {
  return program.splits.length === 0;
}

/**
 * splitBody は PUT /api/program/split のボディを組む。
 *
 * プリセットを渡さなければ分割なしに戻る。
 */
export function splitBody(preset: SplitPreset | null): { splits: SplitPreset['splits'] } {
  return { splits: preset ? preset.splits : [] };
}

/**
 * isSplitSelectable は、その頻度でプリセットを選べるかを返す。
 *
 * 下限は preset.min_frequency_per_week から読む（0 は下限なし）。
 * 「five_way」をここに書かないのは、下限を持つプリセットが増えても
 * 画面側を直さずに済ませるため（サーバー側も同じ理由で seed.SplitPreset
 * に持たせている。CLAUDE.md「必要になるまで作らない」）。
 *
 * 補助の割り振り（PR #185）の計測で、5分割は週4回未満だと部位ごとの
 * 週目標を満たせない。選べる設定を残したまま本人に判断を押し戻すのでは
 * なく、選べる選択肢から外す。
 *
 * 下限0（下限なし）を別条件で弾かない。頻度は1以上しか無い（週の頻度の
 * 選択肢に0は無い）ので、下限0との比較は常に真になり、素通しと同じになる
 * （internal/application/usecase/set_split_cycle.go と同じ判断）。
 */
export function isSplitSelectable(preset: SplitPreset, perWeek: number): boolean {
  return perWeek >= preset.min_frequency_per_week;
}

/**
 * splitUnselectableReason は選べない理由の1行。選べるなら null。
 *
 * サーバーが PUT /api/program/split / /api/program/frequency を拒否する
 * ときと同じ下限を、押す前に画面へ出すためのもの。実際に弾かれる理由は
 * サーバーの応答（describePutFailure）が持つので、ここは「なぜ選べないか」
 * だけを言う。
 */
export function splitUnselectableReason(preset: SplitPreset, perWeek: number): string | null {
  if (isSplitSelectable(preset, perWeek)) return null;
  return `${preset.name}は週${preset.min_frequency_per_week}回以上で使えます`;
}
