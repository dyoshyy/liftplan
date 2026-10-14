import type { CardPlan } from './ExerciseCard';

/**
 * slotCount はカードに出すセットの枠の数。recorded は今日記録済みのセット数。
 *
 * - 自分で選んだ種目: 終わりを決めないので、記録の数より1つ多く。記録のたびに
 *   次の空き枠が出る。何セットやるかは本人が決める
 * - 予定: 予定の数。予定より多く記録していたら、はみ出した分も出す。出さないと
 *   はみ出したセットが画面から消えて、取り消すこともできなくなる
 */
export function slotCount(plan: Pick<CardPlan, 'sets' | 'adhoc'>, recorded: number): number {
  if (plan.adhoc) return recorded + 1;
  return Math.max(plan.sets, recorded);
}
