import { useCallback, useEffect, useState } from 'react';
import { fromState, type Route } from './route';

// useRoute は行き先を持ち、戻るジェスチャーに応える。
//
// 「今日」を土台にして、そこから移った先を履歴に積む。戻ると必ず今日へ
// 帰ってくる。積まずに状態だけで切り替えると、設定を開いたあとの戻るで
// アプリが閉じる。
export function useRoute() {
  const [route, setRoute] = useState<Route>('today');

  useEffect(() => {
    const onPop = (e: PopStateEvent) => setRoute(fromState(e.state));
    window.addEventListener('popstate', onPop);
    return () => window.removeEventListener('popstate', onPop);
  }, []);

  const go = useCallback((next: Route) => {
    setRoute((current) => {
      if (current === next) return current;

      // 土台へ帰るときは積まずに戻す。積むと、今日 → 設定 → 今日 と
      // 移ったあとに戻るを2回押さないとアプリを抜けられない。
      if (next === 'today') {
        history.back();
        return current; // popstate が来たときに切り替わる
      }

      if (current === 'today') history.pushState({ route: next }, '');
      else history.replaceState({ route: next }, '');
      return next;
    });
  }, []);

  return { route, go };
}
