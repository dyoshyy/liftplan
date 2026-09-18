// セッションの状態遷移。純粋関数なので、DOM を立てずに検査できる。
//
// **useReducer を使う本当の理由はテストにある。**以前は hasToken と
// status が別々の useState にあり、「認証に落ちたらトークンを手放す」が
// useEffect で status を見張る形になっていた。遷移が2箇所に割れていて、
// どちらか一方だけ直すと画面が固まる。ここに集めると、遷移そのものを
// 表として検査できる。

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
