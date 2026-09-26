export type SetLike = { weight_kg: number; reps: number };

// formatSets はその日のセットを1行にする。
//
// 全部同じ重量なら「100kg × 8, 8, 8」とまとめる。途中で重量を変えたら
// まとめられないので重量ごとに区切る。1セット目の重量で代表させると、
// 落とした重量も上げた重量も履歴から消える。
export function formatSets(sets: readonly SetLike[]): string {
  const groups: { kg: number; reps: number[] }[] = [];
  for (const s of sets) {
    const tail = groups[groups.length - 1];
    if (tail && tail.kg === s.weight_kg) tail.reps.push(s.reps);
    else groups.push({ kg: s.weight_kg, reps: [s.reps] });
  }
  return groups.map((g) => `${g.kg}kg × ${g.reps.join(', ')}`).join('　/　');
}

export type LastPerformance = {
  weight_kg: number;
  weights?: number[];
  reps: number[];
  days_ago: number;
};

// formatLast は「前回」の1行。formatSets と同じ規則で畳む。
export function formatLast(last: LastPerformance): string {
  const w = last.weights ?? [];
  return formatSets(last.reps.map((r, i) => ({ weight_kg: w[i] ?? last.weight_kg, reps: r })));
}

export type SetValues = { weight: number; reps: number; rir: number };

export type ParsedSetInput = { ok: true; values: SetValues } | { ok: false; warning: string };

/** parseSetInput は入力欄の文字列を1セット分の値にする。読めなければ一言を返す。
 *
 *  今日の記録シートと履歴の編集シートが共有する。検証が画面ごとにずれると、
 *  今日は弾かれる値が履歴からは入る。
 *
 *  RIR は 0 を通す（限界まで追い込んだセット）。重量とレップの 0 は通さない。
 *  推定1RM が 0 に引きずられる。 */
export function parseSetInput(weight: string, reps: string, rir: string): ParsedSetInput {
  const w = Number.parseFloat(weight);
  const r = Number.parseInt(reps, 10);
  const i = Number.parseInt(rir, 10);
  if (!Number.isFinite(w) || !Number.isInteger(r) || !Number.isInteger(i)) {
    return { ok: false, warning: '重量・レップ・RIR を入れてください' };
  }
  if (w <= 0 || r <= 0 || i < 0) {
    return { ok: false, warning: '0 より大きい重量とレップを入れてください' };
  }
  return { ok: true, values: { weight: w, reps: r, rir: i } };
}
