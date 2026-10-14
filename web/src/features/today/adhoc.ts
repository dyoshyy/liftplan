import type { PlannedSet, RecordedSet } from '../../api/types';
import type { CardPlan } from './ExerciseCard';

// 自分で選んだ種目の初期RIR。補助の目標と同じ。
//
// 記録シートの初期値にしか使わない。選んだ種目には目標が無い（何kgでやるか
// を決めるのは本人）ので、処方として画面には出さない。
export const ADHOC_RIR = 2;

/**
 * plannedIds は今日の予定に出ている種目。種目を選ぶシートから除くのに使う。
 *
 * 予定は3レーンすべてを渡すこと（1レーンでも渡し忘れると、
 * その種目が選択肢に残り、選ぶと同じカードが2枚並ぶ）。
 */
export const plannedIds = (planned: readonly (readonly PlannedSet[])[]): Set<string> =>
  new Set(planned.flat().map((p) => p.exercise_id));

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
    .map((id) => ({
      exercise_id: id,
      weight_kg: null,
      sets: 0,
      target_rir: ADHOC_RIR,
      target_reps: 0,
      adhoc: true,
    }));
}

/**
 * belowPlan は予定のカードの下に出す、本人が選んだ種目のカード。
 *
 * 選んだ種目に加えて、今日やったのに予定に無い種目も入れる。選んだ状態は画面の
 * メモリにしか無く、開き直すと消える。そこで記録の数だけの枠にしていたら、
 * 続きを記録する空き枠が無くなった。予定に無い種目をやったのは本人が選んだ
 * からなので、記録から選んだことにする。
 *
 * 予定は3レーンすべてを渡すこと。1レーンでも渡し忘れると、そのレーンの記録が
 * 選んだ種目として二重に出る。
 */
export function belowPlan(
  planned: readonly (readonly PlannedSet[])[],
  picked: readonly string[],
  doneToday: ReadonlyMap<string, readonly RecordedSet[]>,
): CardPlan[] {
  const recorded = [...doneToday.entries()]
    .filter(([, sets]) => sets.length > 0)
    .map(([id]) => id)
    .sort();
  return adhocCards(recorded.reduce(withPicked, [...picked]), planned);
}
