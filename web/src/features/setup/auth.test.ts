import { describe, expect, it } from 'vitest';
import { applyTokenIntake, loginUrl, planTokenIntake, tokenFromHash, type IntakePorts } from './auth';

describe('tokenFromHash', () => {
  it.each([
    { name: 'フラグメントにトークンがある', hash: '#token=abc123', want: 'abc123' },
    { name: '先頭の # が無くても読む', hash: 'token=abc123', want: 'abc123' },
    // サーバーが何を足すか分からない。token だけを見て、他は無視する。
    { name: '他のパラメータが混ざっていても拾う', hash: '#state=xy&token=abc123', want: 'abc123' },
    { name: 'token のあとに別のものが続いても拾う', hash: '#token=abc123&expires=90', want: 'abc123' },
    // URL エンコードされて届く。素の文字列として切り出すと、+ や / を
    // 含むトークンが別物になり、通らないトークンを保存して 401 になる。
    { name: 'URL エンコードを戻す', hash: '#token=a%2Fb%2Bc', want: 'a/b+c' },
    { name: '+ は空白ではなくトークンの一部', hash: '#token=a%2Bb', want: 'a+b' },
    { name: 'フラグメントが無い', hash: '', want: null },
    { name: '# だけ', hash: '#', want: null },
    { name: '値が空', hash: '#token=', want: null },
    { name: '空白だけの値', hash: '#token=%20%20', want: null },
    { name: '別のフラグメント（見出しへのリンクなど）', hash: '#today', want: null },
    { name: '名前が違う', hash: '#access_token=abc123', want: null },
  ])('$name', ({ hash, want }) => {
    expect(tokenFromHash(hash)).toBe(want);
  });
});

describe('loginUrl', () => {
  // サーバーとの約束。ここを変えると、どこにも飛ばないリンクになる。
  it.each([
    { provider: 'github' as const, want: 'https://api.example/auth/github/start' },
    { provider: 'google' as const, want: 'https://api.example/auth/google/start' },
  ])('$provider は /auth/$provider/start へ送る', ({ provider, want }) => {
    expect(loginUrl('https://api.example', provider)).toBe(want);
  });
});

describe('planTokenIntake', () => {
  it('トークンが無ければ何もしない', () => {
    expect(planTokenIntake('#today')).toEqual([]);
  });

  // 順序に意味がある。
  //
  // 保存より先にログイン済みにすると、読み込みが走ったときにトークンが
  // まだ無く、そのまま 401 でログイン画面へ弾き返される。
  //
  // URL から消すのをログイン済みより後にすると、その間に再読み込みが
  // 入ったときに同じフラグメントをもう一度取り込む。401 でトークンを
  // 捨てたあとなら、捨てたはずの古いトークンが URL から蘇る。
  it('保存 → URL から消す → ログイン済み の順で返す', () => {
    expect(planTokenIntake('#token=abc123')).toEqual([
      { kind: 'saveToken', token: 'abc123' },
      { kind: 'clearHash' },
      { kind: 'signIn' },
    ]);
  });
});

describe('applyTokenIntake', () => {
  const recorder = () => {
    const calls: string[] = [];
    const ports: IntakePorts = {
      saveToken: (t) => calls.push(`saveToken:${t}`),
      clearHash: () => calls.push('clearHash'),
      signIn: () => calls.push('signIn'),
    };
    return { calls, ports };
  };

  it('返ってきた順にそのまま実行する', () => {
    const { calls, ports } = recorder();
    applyTokenIntake(planTokenIntake('#token=abc123'), ports);
    expect(calls).toEqual(['saveToken:abc123', 'clearHash', 'signIn']);
  });

  it('何も無ければ1つも呼ばない', () => {
    const { calls, ports } = recorder();
    applyTokenIntake(planTokenIntake(''), ports);
    expect(calls).toEqual([]);
  });
});
