import type { PlannedSet } from '../../api/types';
import type { CardPlan } from './ExerciseCard';

const OFFLINE_MESSAGE = 'オフラインでは出せません';
const COMMUNICATION_ERROR_MESSAGE = '代わりを読み込めませんでした';

/**
 * Swap は予定の補助1つぶんの差し替え。
 *
 * replace は予定に出ていた補助で、何度押してもサーバーへはこれを送る
 * （代わりの代わりを頼むときに、代わりを replace にすると予定に無いので 400）。
 * shown はこれまでに出した代わりで、次に押したとき rejected として送る。
 * current はいま出している代わり。null は「代わりなし」。
 */
export type Swap = { replace: string; shown: string[]; current: PlannedSet | null };

/** SwapState は差し替えと、それがどの日の計画に対するものか。 */
export type SwapState = { date: string; swaps: ReadonlyMap<string, Swap> };

/** swapsFor は date の計画に対する差し替え。別の日のものは持ち越さない。 */
export function swapsFor(state: SwapState, date: string): ReadonlyMap<string, Swap> {
  return state.date === date ? state.swaps : new Map();
}

/**
 * rejectedOf は replace の代わりを次に頼むときに外す種目。
 *
 * replace についてこれまでに出した代わりと、ほかのカードにいま出ている代わり。
 * サーバーは今日の補助を元の計画のまま置いて選ぶので、ほかのカードの代わりは
 * 知らない。外さないと、2台埋まっていた日に同じ代わりが2枚並ぶ。
 */
export function rejectedOf(swaps: ReadonlyMap<string, Swap>, replace: string): string[] {
  const out = new Set(swaps.get(replace)?.shown ?? []);
  for (const s of swaps.values()) {
    if (s.replace !== replace && s.current) out.add(s.current.exercise_id);
  }
  return [...out];
}

/** withResult はサーバーの答えを差し替えに反映した新しい状態を返す。 */
export function withResult(
  state: SwapState,
  date: string,
  replace: string,
  result: PlannedSet | null,
): SwapState {
  const swaps = new Map(swapsFor(state, date));
  const shown = [...(swaps.get(replace)?.shown ?? [])];
  if (result) shown.push(result.exercise_id);
  swaps.set(replace, { replace, shown, current: result });
  return { date, swaps };
}

/**
 * accessoryCards は補助のレーンに出すカード。差し替えた補助はその場所に
 * 代わりが入り、代わりなしと分かった補助には印が付く。
 *
 * ここで作った並びを今日の予定として belowPlan・plannedIds に渡すこと。
 * 渡さないと、代わりを記録した時点で「選んだ種目」にも同じ種目が出る。
 */
export function accessoryCards(
  accessories: readonly PlannedSet[],
  swaps: ReadonlyMap<string, Swap>,
): CardPlan[] {
  return accessories.map((a) => {
    const swap = swaps.get(a.exercise_id);
    if (!swap) return a;
    if (swap.current === null) return { ...a, no_substitute: true };
    return { ...swap.current, substitute_for: a.exercise_id };
  });
}

/**
 * canMarkBlocked は「代替種目」を出すか。補助のカードにだけ使う。
 *
 * 1セットでも記録したら出さない。途中までやったならそこでやめればよい
 * （今日のリストはキュー）。差し替えると同じ枠で2種目を数えることになる。
 */
export function canMarkBlocked(plan: CardPlan, recordedCount: number): boolean {
  return !plan.no_substitute && recordedCount === 0;
}

/**
 * substituteQuery は代わりを頼む URL。date は表示している計画の日付を渡す
 * （端末の「いま」を渡すと、日付をまたいだとき翌日の計画を引いて 400 になる）。
 */
export function substituteQuery(date: string, replace: string, rejected: readonly string[]): string {
  const q = new URLSearchParams({ date, replace });
  for (const id of rejected) q.append('rejected', id);
  return `/api/sessions/substitute?${q.toString()}`;
}

/** StoredSwaps は localStorage に置く形。Map は JSON にできないので配列にする。 */
export type StoredSwaps = { date: string; swaps: Swap[] };

/** encodeSwaps は差し替えを localStorage に置ける形にする。 */
export function encodeSwaps(state: SwapState): StoredSwaps {
  return { date: state.date, swaps: [...state.swaps.values()] };
}

const isString = (v: unknown): v is string => typeof v === 'string';
const isNumber = (v: unknown): v is number => typeof v === 'number' && Number.isFinite(v);

function isPlannedSet(v: unknown): v is PlannedSet {
  if (typeof v !== 'object' || v === null) return false;
  const p = v as Record<string, unknown>;
  return (
    isString(p.exercise_id) &&
    (p.weight_kg === null || isNumber(p.weight_kg)) &&
    isNumber(p.sets) &&
    isNumber(p.target_rir) &&
    isNumber(p.target_reps)
  );
}

function isSwap(v: unknown): v is Swap {
  if (typeof v !== 'object' || v === null) return false;
  const s = v as Record<string, unknown>;
  return (
    isString(s.replace) &&
    Array.isArray(s.shown) &&
    s.shown.every(isString) &&
    (s.current === null || isPlannedSet(s.current))
  );
}

/**
 * decodeSwaps は localStorage から読んだ値を差し替えに戻す。
 *
 * 形が1か所でも合わなければ全部捨てて空から始める。古い版が残した値や
 * 手で壊れた値で画面を止めない。捨てても失うのは差し替えだけで、記録は
 * 残る（「選んだ種目」に出る）。
 */
export function decodeSwaps(raw: unknown): SwapState {
  const none: SwapState = { date: '', swaps: new Map() };
  if (typeof raw !== 'object' || raw === null) return none;
  const r = raw as Record<string, unknown>;
  if (!isString(r.date) || !Array.isArray(r.swaps) || !r.swaps.every(isSwap)) return none;
  return { date: r.date, swaps: new Map(r.swaps.map((s: Swap) => [s.replace, s])) };
}

/** substituteErrorMessage は取得に失敗したときの一言（forecastErrorMessage と同じ分け方）。 */
export function substituteErrorMessage(e: unknown): string {
  return e instanceof TypeError ? OFFLINE_MESSAGE : COMMUNICATION_ERROR_MESSAGE;
}
