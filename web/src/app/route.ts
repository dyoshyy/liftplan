// 画面の行き先。ルーターは入れない。
//
// 行き先は3つで、パラメータもネストも無い。ルーターを足すと、URL の設計と
// 型と再描画の規則が増えるだけで、返ってくるものが無い。
//
// ただし**戻るジェスチャーだけは扱う**。Android では画面端のスワイプが
// 「1つ前へ」の主要な操作で、状態だけで画面を切り替えると、設定を開いた
// あとに戻ろうとしてアプリごと閉じる。PWA では致命的で、記録の途中なら
// なおさら困る。

export type Route = 'today' | 'history' | 'settings' | 'forecast' | 'exercises';

export const isRoute = (v: unknown): v is Route =>
  v === 'today' || v === 'history' || v === 'settings' || v === 'forecast' || v === 'exercises';

/** fromState は popstate が運んできた値を Route に戻す。
 *  他所が積んだ履歴や壊れた値でも、必ず既定へ倒す。 */
export const fromState = (state: unknown): Route => {
  const route = (state as { route?: unknown } | null)?.route;
  return isRoute(route) ? route : 'today';
};

/** parentOf は1段上の行き先。種目ページは設定から開く子。無ければ今日の直下。 */
const parentOf: Partial<Record<Route, Route>> = { exercises: 'settings' };

/** depth は今日から何段積んだ位置か。今日が 0。 */
const depth = (r: Route): number => {
  if (r === 'today') return 0;
  const p = parentOf[r];
  return p ? depth(p) + 1 : 1;
};

/** Move は行き先を移るときの履歴の扱い。 */
export type Move =
  | { kind: 'stay' }
  /** push は1段積む。 */
  | { kind: 'push' }
  /** replace は今の段を置き換える。 */
  | { kind: 'replace' }
  /** back は steps 段戻る。then があれば、着いた段をそれに置き換える。 */
  | { kind: 'back'; steps: number; then?: Route };

/**
 * planMove は current から next へ移るときの履歴の扱いを決める。
 *
 * 履歴は「今日 → 画面 → 子の画面」の段そのものにする。戻るジェスチャーが
 * 1段上へ、今日で戻ればアプリを抜ける。今日や上の段へは積まずに戻る（積むと、
 * 戻るを余計に押さないと抜けられない）。
 */
export function planMove(current: Route, next: Route): Move {
  if (current === next) return { kind: 'stay' };
  if (parentOf[next] === current || current === 'today') return { kind: 'push' };

  // next が current の上の段なら、その段まで戻る。
  for (let r: Route | undefined = current, steps = 0; r !== undefined;) {
    if (r === next) return { kind: 'back', steps };
    if (r === 'today') break;
    r = parentOf[r] ?? 'today';
    steps++;
  }

  // 別の枝。今日の直下の段まで戻ってから置き換える。
  const steps = depth(current) - 1;
  return steps === 0 ? { kind: 'replace' } : { kind: 'back', steps, then: next };
}
