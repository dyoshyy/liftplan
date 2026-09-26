import { useCallback, useEffect, useReducer, useRef } from 'react';
import { getToken, setToken } from '../storage/local';
import { applyTokenIntake, planTokenIntake } from '../features/setup/auth';
import { useLiftplan } from './useLiftplan';
import { useOutbox } from './useOutbox';

/** LoadState は読み込みの結果。電波の有無とは別物。 */
export type LoadState = 'loading' | 'ready' | 'offline' | 'unauthorized';

export type SessionState = {
  hasToken: boolean;
  load: LoadState;
  online: boolean;
};

export type SessionAction =
  | { type: 'SIGNED_IN' }
  | { type: 'SIGNED_OUT' }
  | { type: 'LOAD_STARTED' }
  | { type: 'LOAD_SUCCEEDED' }
  | { type: 'LOAD_FAILED'; reason: 'offline' | 'unauthorized' }
  | { type: 'WENT_ONLINE' }
  | { type: 'WENT_OFFLINE' };

export const initialSession = (hasToken: boolean): SessionState => ({
  hasToken,
  // トークンが無ければ読みに行かない。読みに行っても 401 が返るだけで、
  // その 401 がトークンを消しにかかる。
  load: hasToken ? 'loading' : 'ready',
  online: true,
});

export function sessionReducer(state: SessionState, action: SessionAction): SessionState {
  switch (action.type) {
    case 'SIGNED_IN':
      return { ...state, hasToken: true, load: 'loading' };

    case 'SIGNED_OUT':
      return { ...state, hasToken: false, load: 'ready' };

    case 'LOAD_STARTED':
      return state.load === 'loading' ? state : { ...state, load: 'loading' };

    case 'LOAD_SUCCEEDED':
      return { ...state, load: 'ready' };

    case 'LOAD_FAILED':
      // 認証に落ちたときだけトークンを手放す。つながらないだけなら持ったまま。
      // 圏外で開くたびにトークンが消えたら、ジムで入力し直すことになる。
      return {
        ...state,
        load: action.reason,
        hasToken: action.reason === 'unauthorized' ? false : state.hasToken,
      };

    // 電波の有無は読み込みの成否を変えない。圏外になっただけで
    // 「読めていない」ことにすると、読み終えた画面が消える。
    case 'WENT_ONLINE':
      return state.online ? state : { ...state, online: true };

    case 'WENT_OFFLINE':
      return state.online ? { ...state, online: false } : state;
  }
}

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
  const { data, loadAll, loadToday, recordLocally, forgetLocally, applyLocally } = useLiftplan();

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

  // ログインから戻ってきたときの取り込み。
  //
  // 何をどの順でやるかは planTokenIntake が決めている。ここは順に実行する
  // だけで、並べ替えない。history.state をそのまま渡し直すのは、
  // useRoute が積んだ行き先を消さないため（消すと戻るでアプリが閉じる）。
  useEffect(() => {
    applyTokenIntake(planTokenIntake(location.hash), {
      saveToken: setToken,
      clearHash: () => history.replaceState(history.state, '', location.pathname + location.search),
      signIn: () => dispatch({ type: 'SIGNED_IN' }),
    });
  }, []);

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
    applyLocally,
    signOut,
  };
}
