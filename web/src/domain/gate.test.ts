import { describe, expect, it } from 'vitest';
import { createGate } from './gate';

// 解決を外から握る。実行中の状態を作るのに使う。
const deferred = () => {
  let resolve!: () => void;
  let reject!: (e: Error) => void;
  const promise = new Promise<void>((res, rej) => {
    resolve = res;
    reject = rej;
  });
  return { promise, resolve, reject };
};

describe('createGate', () => {
  // 記録は待ち行列への書き込みを待つ。待っている間もシートは開いたままなので、
  // 連打すると同じ手順がもう一度走り、別のIDで二重に記録される。
  it('実行中に来た呼び出しは捨てる', async () => {
    const gate = createGate();
    const d = deferred();
    let calls = 0;
    const work = async () => {
      calls++;
      await d.promise;
    };

    const first = gate(work);
    await gate(work);
    d.resolve();
    await first;

    expect(calls).toBe(1);
  });

  // 人の連打だけでなく、同じ tick に2回呼ばれる場合も止める。
  // 「実行中」の印を最初の await の前に立てていないと、2回目が素通りする。
  it('同じ tick の2回目も捨てる', async () => {
    const gate = createGate();
    let calls = 0;
    const work = async () => {
      calls++;
      await Promise.resolve();
    };

    await Promise.all([gate(work), gate(work)]);

    expect(calls).toBe(1);
  });

  // 門が開かないままだと、1回記録しただけで以後の記録が全部捨てられる。
  it('終わったら次の呼び出しを通す', async () => {
    const gate = createGate();
    let calls = 0;
    const work = async () => {
      calls++;
    };

    await gate(work);
    await gate(work);

    expect(calls).toBe(2);
  });

  // 保存に失敗したあとも門が閉じたままだと、やり直しが永久に効かない。
  it('失敗しても門は開き、失敗はそのまま呼び手に返る', async () => {
    const gate = createGate();
    const d = deferred();
    const failing = async () => {
      await d.promise;
    };
    const first = gate(failing);
    d.reject(new Error('書き込めない'));
    await expect(first).rejects.toThrow('書き込めない');

    let calls = 0;
    await gate(async () => {
      calls++;
    });
    expect(calls).toBe(1);
  });

  // 門は呼び手ごとに別。1つの画面の門が、別の画面の操作を止めない。
  it('別の門は互いに止めない', async () => {
    const a = createGate();
    const b = createGate();
    const d = deferred();
    let calls = 0;

    const first = a(async () => {
      await d.promise;
    });
    await b(async () => {
      calls++;
    });
    d.resolve();
    await first;

    expect(calls).toBe(1);
  });
});
