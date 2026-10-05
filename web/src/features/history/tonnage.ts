import type { Day } from '../../api/types';

/** dayTonnage はその日の総挙上量（重量×回数の合計、kg）。 */
export const dayTonnage = (day: Day): number =>
  day.exercises.reduce((a, e) => a + e.sets.reduce((b, s) => b + s.weight_kg * s.reps, 0), 0);

/** formatTonnage は1万kg を超えたら t にする。桁が多いと月の見出しで読めない。 */
export function formatTonnage(kg: number): string {
  if (kg >= 10_000) return `${(kg / 1000).toFixed(1)}t`;
  return `${Math.round(kg).toLocaleString('en-US')}kg`;
}
