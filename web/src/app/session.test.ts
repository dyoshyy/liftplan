import { describe, expect, it } from 'vitest';
import { initialSession, sessionReducer, type SessionState } from './session';

const signedIn: SessionState = { hasToken: true, load: 'ready', online: true };

describe('sessionReducer', () => {
  it('トークンを入れたら読み込みから始める', () => {
    const got = sessionReducer({ hasToken: false, load: 'ready', online: true }, { type: 'SIGNED_IN' });
    expect(got).toEqual({ hasToken: true, load: 'loading', online: true });
  });

  // トークンが通らなくなったら設定へ戻す。以前は useEffect で status を
  // 見張って setHasToken していたので、遷移が2箇所に割れていた。
  it('認証に落ちたらトークンを手放す', () => {
    expect(sessionReducer(signedIn, { type: 'LOAD_FAILED', reason: 'unauthorized' })).toEqual({
      hasToken: false,
      load: 'unauthorized',
      online: true,
    });
  });

  it('つながらないだけならトークンは持ったまま', () => {
    expect(sessionReducer(signedIn, { type: 'LOAD_FAILED', reason: 'offline' })).toEqual({
      hasToken: true,
      load: 'offline',
      online: true,
    });
  });

  it('トークンを消したら設定へ戻る', () => {
    expect(sessionReducer(signedIn, { type: 'SIGNED_OUT' }).hasToken).toBe(false);
  });

  it.each([
    { type: 'WENT_ONLINE' as const, want: true },
    { type: 'WENT_OFFLINE' as const, want: false },
  ])('$type で online が $want になる', ({ type, want }) => {
    expect(sessionReducer({ ...signedIn, online: !want }, { type }).online).toBe(want);
  });

  // 電波の有無は読み込みの成否と別物。圏外になっただけで
  // 「読めていない」ことにすると、読み終えた画面が消える。
  it('オフラインになっても読み込みの結果は変えない', () => {
    expect(sessionReducer(signedIn, { type: 'WENT_OFFLINE' }).load).toBe('ready');
  });

  it('知らない遷移では同じものを返す（再描画を起こさない）', () => {
    const state = { ...signedIn };
    expect(sessionReducer(state, { type: 'WENT_ONLINE' })).toBe(state);
  });
});

describe('initialSession', () => {
  it('トークンがあれば読み込みから始まる', () => {
    expect(initialSession(true)).toEqual({ hasToken: true, load: 'loading', online: true });
  });

  it('トークンが無ければ読みに行かない', () => {
    expect(initialSession(false).load).toBe('ready');
  });
});
