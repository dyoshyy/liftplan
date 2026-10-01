import type { Exercise, PlannedSet, RecordedSet } from '../../api/types';
import type { CardPlan } from './ExerciseCard';
import { leftovers } from './leftovers';

// 自分で選んだ種目の初期RIR。補助の目標と同じ。
//
// 記録シートの初期値にしか使わない。選んだ種目には目標が無い（何kgでやるか
// を決めるのは本人）ので、処方として画面には出さない。
export const ADHOC_RIR = 2;

const plannedIds = (planned: readonly (readonly PlannedSet[])[]) =>
  new Set(planned.flat().map((p) => p.exercise_id));

/**
 * pickableExercises は「種目を選んで記録」の選択肢。
 *
 * 今日の予定に出ている種目は入れない。選んでも同じカードが2枚並ぶだけで、
 * 予定のカードに記録すれば済む。予定は3レーンすべてを渡すこと（leftovers と
 * 同じ理由。1レーンでも渡し忘れると、その種目が選択肢に残る）。
 *
 * 並びはサーバーが返した順のまま。名前の一部で絞れる（前後の空白と
 * 英字の大文字小文字は無視する）。
 */
export function pickableExercises(
  exercises: readonly Exercise[],
  planned: readonly (readonly PlannedSet[])[],
  query: string,
): Exercise[] {
  const ids = plannedIds(planned);
  const q = query.trim().toLowerCase();
  return exercises.filter((e) => !ids.has(e.id) && e.name.toLowerCase().includes(q));
}

/** withPicked は選んだ種目を足す。選び済みなら足さない（同じカードが2枚出る）。 */
export function withPicked(picked: readonly string[], id: string): string[] {
  return picked.includes(id) ? [...picked] : [...picked, id];
}

/**
 * adhocCards は選んだ種目のカード。重量は未定、セット数は0。
 *
 * 選んだあとで予定に入った種目は返さない。予定のカードに任せる。
 */
export function adhocCards(
  picked: readonly string[],
  planned: readonly (readonly PlannedSet[])[],
): CardPlan[] {
  const ids = plannedIds(planned);
  return picked
    .filter((id) => !ids.has(id))
    .map((id) => ({ exercise_id: id, weight_kg: null, sets: 0, target_rir: ADHOC_RIR, target_reps: 0, adhoc: true }));
}

/**
 * belowPlan は予定のカードの下に出す2組。本人が選んだ種目と、今日やったもの。
 *
 * 選んだ種目は予定として数えて leftovers に渡す。渡さないと、記録した時点で
 * 「今日やったもの」にも出て、同じ種目が2枚並ぶ。
 */
export function belowPlan(
  planned: readonly (readonly PlannedSet[])[],
  picked: readonly string[],
  doneToday: ReadonlyMap<string, readonly RecordedSet[]>,
): { adhoc: CardPlan[]; done: CardPlan[] } {
  const adhoc = adhocCards(picked, planned);
  return { adhoc, done: leftovers([...planned, adhoc], doneToday) };
}
