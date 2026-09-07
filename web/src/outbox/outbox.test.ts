import { beforeEach, describe, expect, it } from 'vitest';
import 'fake-indexeddb/auto';
import { Outbox } from './outbox';
import type { QueueItem } from './db';
import { clearAll } from './db';

// ok は成功の応答。send を差し替えられるようにしてあるので、
// ネットワークを立てずに再送の規則を検査できる。
const ok = () => new Response('', { status: 200 });
const status = (code: number) => () => new Response('', { status: code });

/** recorder は送られたものを順に控える送信関数を作る。 */
function recorder(reply: (item: QueueItem) => Response = ok) {
  const sent: QueueItem[] = [];
  return {
    sent,
    send: async (item: QueueItem) => {
      sent.push(item);
      return reply(item);
    },
  };
}

/** tick はマクロタスクを数回まわす。IndexedDB の読み取りが挟まるので、
 *  マイクロタスクだけでは相手側が進まない。 */
const tick = async () => {
  for (let i = 0; i < 5; i++) await new Promise((r) => setTimeout(r, 0));
};

const setLog = (id: string): QueueItem => ({
  path: '/api/set-logs',
  body: { logs: [{ id, date: '2026-09-07', exercise_id: 'bench', weight_kg: 100, reps: 8, rir: 2 }] },
});

describe('Outbox', () => {
  beforeEach(async () => {
    await clearAll();
  });

  it('積んだものを順に送り、送れたら待ち行列から外す', async () => {
    const r = recorder();
    const outbox = new Outbox(r.send);

    await outbox.enqueue(setLog('a'));
    await outbox.enqueue(setLog('b'));
    await outbox.flush();

    expect(r.sent.map((i) => (i.body as { logs: { id: string }[] }).logs[0]!.id)).toEqual(['a', 'b']);
    expect(await outbox.pendingCount()).toBe(0);
  });

  // 4xx は再送しても永久に通らない。残すと後続が全部詰まる。
  // ただし黙って消すと、記録したはずのものが無いことに気づけない。
  it('4xx は rejected へ移し、後続を通す', async () => {
    const r = recorder((item) =>
      (item.body as { logs: { id: string }[] }).logs[0]!.id === 'a'
        ? new Response('', { status: 400 })
        : new Response('', { status: 200 }),
    );
    const outbox = new Outbox(r.send);

    await outbox.enqueue(setLog('a'));
    await outbox.enqueue(setLog('b'));
    await outbox.flush();

    expect(await outbox.pendingCount()).toBe(0);
    expect(await outbox.rejected()).toHaveLength(1);
    expect(r.sent).toHaveLength(2);
  });

  // 5xx はやり直せば通る。捨てたら記録が消える。
  it('5xx は待ち行列に残し、そこで止める', async () => {
    const r = recorder(status(503));
    const outbox = new Outbox(r.send);

    await outbox.enqueue(setLog('a'));
    await outbox.enqueue(setLog('b'));
    await outbox.flush();

    expect(await outbox.pendingCount()).toBe(2);
    // 先頭で止める。後続を先に送ると、記録の順序が入れ替わる。
    expect(r.sent).toHaveLength(1);
    expect(await outbox.rejected()).toHaveLength(0);
  });

  it('電波が無いときは待ち行列を減らさない', async () => {
    const outbox = new Outbox(async () => {
      throw new TypeError('Failed to fetch');
    });

    await outbox.enqueue(setLog('a'));
    await outbox.flush();

    expect(await outbox.pendingCount()).toBe(1);
  });

  // 同時に2回 flush が走ると、同じ先頭を2回送って二重に記録される。
  //
  // 送信を手で止められるようにして「1件目が飛んでいる最中に」2本目の
  // flush を始める。止めないと、2本目が待ち行列を読み終える前に
  // 1件目が完了してしまい、重なりが起きないまま緑になる。
  it('1件目の送信中に始めた flush は何も送らない', async () => {
    let release: (res: Response) => void = () => {};
    const inFlight = new Promise<Response>((r) => {
      release = r;
    });
    let calls = 0;
    const outbox = new Outbox(async () => {
      calls++;
      return inFlight;
    });

    await outbox.enqueue(setLog('a'));
    await outbox.enqueue(setLog('b'));

    const first = outbox.flush();
    await tick(); // 1件目が send に入るまで進める

    const second = outbox.flush();
    await tick(); // 2本目が待ち行列を読みきるだけの余裕を与える

    expect(calls).toBe(1);

    release(ok());
    await Promise.all([first, second]);
  });

  it('rejected を消せる', async () => {
    const outbox = new Outbox(recorder(status(409)).send);
    await outbox.enqueue(setLog('a'));
    await outbox.flush();
    expect(await outbox.rejected()).toHaveLength(1);

    await outbox.clearRejected();
    expect(await outbox.rejected()).toHaveLength(0);
  });
});
