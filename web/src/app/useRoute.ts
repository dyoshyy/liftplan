import { useCallback, useEffect, useRef, useState } from 'react';
import { fromState, planMove, type Route } from './route';

// useRoute は行き先を持ち、戻るジェスチャーに応える。
//
// 「今日」を土台にして、そこから移った先を履歴に積む。戻ると1段上へ帰り、
// 今日で戻ればアプリを抜ける。積まずに状態だけで切り替えると、設定を開いた
// あとの戻るでアプリが閉じる。どう積むかは planMove（route.ts）が決め、
// ここは実行するだけ。
export function useRoute() {
  const [route, setRoute] = useState<Route>('today');
  // 戻ったあとに置き換える行き先。history.go は非同期で、着いたことは
  // popstate でしか分からない。
  const pending = useRef<Route | null>(null);

  useEffect(() => {
    const onPop = (e: PopStateEvent) => {
      const then = pending.current;
      pending.current = null;
      if (then) {
        history.replaceState({ route: then }, '');
        setRoute(then);
        return;
      }
      setRoute(fromState(e.state));
    };
    window.addEventListener('popstate', onPop);
    return () => window.removeEventListener('popstate', onPop);
  }, []);

  const go = useCallback((next: Route) => {
    setRoute((current) => {
      const move = planMove(current, next);
      switch (move.kind) {
        case 'stay':
          return current;
        case 'push':
          history.pushState({ route: next }, '');
          return next;
        case 'replace':
          history.replaceState({ route: next }, '');
          return next;
        case 'back':
          pending.current = move.then ?? null;
          history.go(-move.steps);
          return current; // popstate が来たときに切り替わる
      }
    });
  }, []);

  return { route, go };
}
