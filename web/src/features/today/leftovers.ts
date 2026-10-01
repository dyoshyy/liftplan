import type { PlannedSet, RecordedSet } from '../../api/types';
import type { CardPlan } from './ExerciseCard';

/**
 * leftovers は今日やったのに、今の予定に入っていないものを返す。
 *
 * 補助種目は終わると枠から外れるので、セッションを終えて開き直すと
 * カードが11枚から3枚に減る。やった24セットが今日の画面から消えて、
 * 記録が飛んだように見える。
 *
 * 予定は3レーンすべてを渡すこと。1レーンでも渡し忘れると、その種目が
 * 「今日やったもの」として二重に出る。レーンが増えたときに落としやすい
 * ので、関数に切り出してテストで押さえている。
 */
export function leftovers(
  planned: readonly (readonly PlannedSet[])[],
  doneToday: ReadonlyMap<string, readonly RecordedSet[]>,
): CardPlan[] {
  const ids = new Set(planned.flat().map((p) => p.exercise_id));
  return [...doneToday.entries()]
    .filter(([id, sets]) => !ids.has(id) && sets.length > 0)
    .sort((a, b) => (a[0] < b[0] ? -1 : 1))
    .map(([id, sets]) => ({
      exercise_id: id,
      weight_kg: null,
      sets: sets.length,
      target_rir: 0,
      target_reps: 0,
      finished_only: true,
    }));
}
