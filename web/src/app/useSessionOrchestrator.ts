import { useCallback, useEffect, useReducer, useRef } from 'react';
import { getToken } from '../storage/local';
import { initialSession, sessionReducer } from './session';
import { useLiftplan } from './useLiftplan';
import { useOutbox } from './useOutbox';

// セッションの調停。読み込み・待ち行列・認証・電波を1箇所で束ねる。
//
// **順序に意味がある。**溜まっているものを先に送りきってから読む。逆に
// すると、送信前の状態で描画してから送ることになり、記録したのに緑が
// 消えて見える。オフラインで記録して復帰したときに必ず踏み、「消えた」と
// 思ってもう一度記録して重複する。
//
// 遷移そのものは session.ts（純粋）にある。ここは順に実行して、
// 結果を dispatch するだけ。
export function useSessionOrchestrator() {
  const [state, dispatch] = useReducer(sessionReducer, getToken() !== '', initialSession);
  const { data, loadAll, loadToday, recordLocally, forgetLocally } = useLiftplan();

  // 捨てた記録の説明に種目名が要る。名前は読み込みで手に入るので、
  // ここで繋ぐ。外から渡す形にすると、名前を持っているのは中なのに
  // 呼び手が用意することになり、循環する。
  const nameOf = useCallback((id: string) => data.names.get(id) ?? id, [data.names]);
  const outbox = useOutbox(nameOf);
  const { flush } = outbox;

  const reload = useCallback(async () => {
    if (getToken() === '') return;
    dispatch({ type: 'LOAD_STARTED' });

    await flush();
    const result = await loadAll();

    if (result.ok) dispatch({ type: 'LOAD_SUCCEEDED' });
    else dispatch({ type: 'LOAD_FAILED', reason: result.reason });
  }, [flush, loadAll]);

  useEffect(() => {
    if (state.hasToken) void reload();
  }, [state.hasToken, reload]);

  // 復帰したら送るだけでなく、メニューも取り直す。
  //
  // 圏外で開くと「つながりません」だけの画面になる。電波が戻っても送信
  // しかしないと、メニューは空のままで、利用者が押すまで今日の内容が
  // 出ない。ジムに着いて開き、電波を掴んだところで何も出ないのは、
  // 壊れているのと区別がつかない。
  //
  // 取り直すのはメニューが無いときだけ。毎回取り直すと、記録の最中に
  // 一瞬電波が切れただけで画面が組み替わる。
  const loadRef = useRef(state.load);
  loadRef.current = state.load;

  useEffect(() => {
    const onOnline = () => {
      dispatch({ type: 'WENT_ONLINE' });
      if (loadRef.current === 'offline') void reload();
      else void flush();
    };
    const onOffline = () => dispatch({ type: 'WENT_OFFLINE' });

    window.addEventListener('online', onOnline);
    window.addEventListener('offline', onOffline);
    return () => {
      window.removeEventListener('online', onOnline);
      window.removeEventListener('offline', onOffline);
    };
  }, [flush, reload]);

  const signIn = useCallback(() => dispatch({ type: 'SIGNED_IN' }), []);
  const signOut = useCallback(() => dispatch({ type: 'SIGNED_OUT' }), []);

  return {
    ...state,
    nameOf,
    data,
    outbox,
    reload,
    loadToday,
    recordLocally,
    forgetLocally,
    signIn,
    signOut,
  };
}
