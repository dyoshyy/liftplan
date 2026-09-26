import type { Day, RecordedSet } from '../../api/types';

// 自己ベストの更新を判定する。
//
// **サーバーには置かない。**自己ベストは「今この瞬間に見せる合図」で、
// 処方の導出には一切入っていない。サーバーに持たせると、記録を送って
// 応答を待つまで演出が出ない。手元の履歴で足りる（D-127 と同じ理由で、
// ジムで開く画面に往復を足さない）。
//
// **比べる相手は直近8週。**`useLiftplan` が読むのが56日ぶんで、それより
// 前は端末に無い。9週前の記録を下回っていても更新として出る。窓を広げる
// なら `/api/set-logs` の from を伸ばすことになるので、必要になってから。

/** PersonalRecord は更新の中身。演出が表示に使う。 */
export type PersonalRecord = {
  exerciseId: string;
  name: string;
  /** estimatedKg は今回の推定1RM。 */
  estimatedKg: number;
  /** previousKg はそれまでの最高。 */
  previousKg: number;
  /** deltaKg は伸びた幅。 */
  deltaKg: number;
};

export type SetValues = { weight: number; reps: number; rir: number };

// maxRepsToFailure は Epley 式を適用してよい「限界までの総レップ数」の上限。
// サーバー（internal/domain/training/one_rep_max.go）と同じ値。ここを
// ずらすと、画面が祝った記録をサーバーが推定1RMとして採らない食い違いが出る。
const MAX_REPS_TO_FAILURE = 20;

const EPLEY_DIVISOR = 30;

// QUANTUM はサーバーの measures.go と同じ量子化の桁。
// 丸めずに比べると 0.30000000000000004 > 0.3 が成り立ち、
// 浮動小数点の残差だけで演出が出る。
const QUANTUM = 1e6;

const quantize = (v: number): number => Math.round(v * QUANTUM) / QUANTUM;

// estimateOneRepMax は1セットから推定1RMを出す。推定できなければ null。
//
// 推定できないのは、0kg の自重種目と、限界までの総レップが適用範囲を
// 超えたセット。後者を通すと 20kg×100レップが 86.7kg の自己ベストになる。
export function estimateOneRepMax(v: SetValues): number | null {
  const repsToFailure = v.reps + v.rir;
  if (repsToFailure > MAX_REPS_TO_FAILURE) return null;
  // 自重種目（0kg）からは1RMを推定できない。0を返すと、重りを付けた
  // 最初の1セットが「0kg からの更新」になる。
  if (!(v.weight > 0)) return null;

  const kg = quantize(v.weight * (1 + repsToFailure / EPLEY_DIVISOR));
  return Number.isFinite(kg) ? kg : null;
}

type PreviousInput = {
  days: readonly Day[];
  doneToday: ReadonlyMap<string, readonly RecordedSet[]>;
  exerciseId: string;
  today: string;
  /** excludeId は直しているセット自身。母集団に残すと自分自身を超えられない。 */
  excludeId?: string;
};

// previousSets は比べる相手を集める。
//
// **今日のぶんは `days` ではなく `doneToday` から取る。**`days` の今日の
// 分と `doneToday` は同じ記録を指す（どちらも applyDayChange が進める）。
// 両方から取ると同じセットが2回入るので片方に寄せる。寄せる先は、今日の
// 画面が記録のたびに見ている `doneToday`。以前は `days` が記録で進まず、
// こちらを見ていたら今日 1セット目の自己ベストが2セット目の判定から消えて、
// 同じ重量でもう一度祝うことになっていた。
export function previousSets({
  days,
  doneToday,
  exerciseId,
  today,
  excludeId,
}: PreviousInput): RecordedSet[] {
  const past = days
    .filter((d) => d.date !== today)
    .flatMap((d) => d.exercises.filter((e) => e.exercise_id === exerciseId))
    .flatMap((e) => e.sets);

  return [...past, ...(doneToday.get(exerciseId) ?? [])].filter((s) => s.id !== excludeId);
}

type JudgeInput = {
  exerciseId: string;
  name: string;
  values: SetValues;
  previous: readonly RecordedSet[];
};

// judgePersonalRecord は今回のセットが自己ベストの更新かを決める。
//
// 比べる相手が1件も無いときは祝わない。使い始めの1セット目で全種目が
// 発火すると、演出が「更新した合図」ではなく「記録したときに出るもの」
// になり、本当に更新した日と区別がつかなくなる。
export function judgePersonalRecord({
  exerciseId,
  name,
  values,
  previous,
}: JudgeInput): PersonalRecord | null {
  const now = estimateOneRepMax(values);
  if (now === null) return null;

  const estimates = previous
    .map((s) => estimateOneRepMax({ weight: s.weight_kg, reps: s.reps, rir: s.rir }))
    .filter((kg): kg is number => kg !== null);
  if (estimates.length === 0) return null;

  const previousKg = Math.max(...estimates);
  if (now <= previousKg) return null;

  return { exerciseId, name, estimatedKg: now, previousKg, deltaKg: quantize(now - previousKg) };
}

// formatKg は表示用の1桁。125.0kg ではなく 125kg と出す。
// 量子化の残差（116.66666666666667）をそのまま出すと、数字が画面から
// はみ出す。
export function formatKg(kg: number): string {
  return kg.toFixed(1).replace(/\.0$/, '');
}
