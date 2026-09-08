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
