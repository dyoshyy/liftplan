// 画面の行き先。ルーターは入れない。
//
// 行き先は3つで、パラメータもネストも無い。ルーターを足すと、URL の設計と
// 型と再描画の規則が増えるだけで、返ってくるものが無い。
//
// ただし**戻るジェスチャーだけは扱う**。Android では画面端のスワイプが
// 「1つ前へ」の主要な操作で、状態だけで画面を切り替えると、設定を開いた
// あとに戻ろうとしてアプリごと閉じる。PWA では致命的で、記録の途中なら
// なおさら困る。

export type Route = 'today' | 'history' | 'settings';

export const isRoute = (v: unknown): v is Route =>
  v === 'today' || v === 'history' || v === 'settings';

/** fromState は popstate が運んできた値を Route に戻す。
 *  他所が積んだ履歴や壊れた値でも、必ず既定へ倒す。 */
export const fromState = (state: unknown): Route => {
  const route = (state as { route?: unknown } | null)?.route;
  return isRoute(route) ? route : 'today';
};
