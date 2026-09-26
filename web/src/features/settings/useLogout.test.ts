import { describe, expect, it } from 'vitest';
import { runLogout, type LogoutPorts } from './useLogout';

// 呼ばれた順を記録するだけの実体。revoke の結末だけ差し替える。
function portsWith(revoke: () => Promise<unknown>) {
  const calls: string[] = [];
  const ports: LogoutPorts = {
    revoke: async () => {
      calls.push('revoke');
      return revoke();
    },
    clearToken: () => calls.push('clearToken'),
    forget: () => calls.push('forget'),
  };
  return { calls, ports };
}

describe('runLogout', () => {
  // 失効の要求にはトークンを付ける。先に捨てると付けるものが無く、
  // サーバーは 401 を返すだけで、セッションは期限（90日）まで生き残る。
  it('サーバーで失効させてから、端末のトークンを捨てる', async () => {
    const { calls, ports } = portsWith(async () => new Response(null, { status: 204 }));
    await runLogout(ports);
    expect(calls).toEqual(['revoke', 'clearToken', 'forget']);
  });

  // 失効に失敗したからといって端末にトークンを残すと、圏外ではログアウト
  // できなくなる。共有の端末から立ち去れないほうが困るので、捨てて先へ進む。
  it.each([
    ['圏外（fetch が落ちる）', () => Promise.reject(new TypeError('Failed to fetch'))],
    ['401（api/client が Unauthorized を投げる）', () => Promise.reject(new Error('unauthorized'))],
    ['5xx（応答は返るが成功ではない）', async () => new Response(null, { status: 503 })],
  ])('失効に失敗しても端末のトークンは捨てる: %s', async (_, revoke) => {
    const { calls, ports } = portsWith(revoke);
    await expect(runLogout(ports)).resolves.toBeUndefined();
    expect(calls).toEqual(['revoke', 'clearToken', 'forget']);
  });
});
