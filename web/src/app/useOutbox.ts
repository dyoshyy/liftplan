import { useCallback, useEffect, useRef, useState } from 'react';
import { Outbox } from '../outbox/outbox';
import { adoptLegacyQueue, type QueueItem } from '../outbox/db';
import { send } from '../api/client';

export type OutboxState = {
  pending: number;
  rejected: string[];
};

// useOutbox は待ち行列を画面につなぐ。
//
// Outbox 自体は React も fetch も知らない。ここが唯一の接点で、
// 送信関数を渡し、件数を state に映すことだけをする。
export function useOutbox(nameOf: (id: string) => string) {
  const outboxRef = useRef<Outbox>(undefined);
  outboxRef.current ??= new Outbox(send);
  const outbox = outboxRef.current;

  // 捨てた記録の説明に種目名を使う。読み込みの後に差し替わる。
  outbox.nameOf = nameOf;

  const [state, setState] = useState<OutboxState>({ pending: 0, rejected: [] });

  const refresh = useCallback(async () => {
    setState({ pending: await outbox.pendingCount(), rejected: await outbox.rejected() });
  }, [outbox]);

  const flush = useCallback(async () => {
    await outbox.flush();
    await refresh();
  }, [outbox, refresh]);

  // enqueueAll は端末に積んだところで返る。**送信の完了は待たない。**
  //
  // 以前は積んだあとに flush を待っていたので、押してから画面が進むまでに
  // サーバーの往復が丸ごと入った（応答が 2 秒遅いと、シートが閉じるまで
  // 2 秒かかった）。端末に積めた時点で記録は失われないので、送信は裏で回す。
  //
  // 戻り値は積めたかどうか。積めなかったとき（容量超過・プライベートモード）に
  // 黙って返すと、呼び手は保存できたつもりで画面を進めてしまう。
  const enqueueAll = useCallback(
    async (items: readonly QueueItem[]): Promise<boolean> => {
      if (items.length === 0) return true;
      // 楽観的に数えておく。押した直後に「未送信」と出ないと、
      // 記録できたのか分からない。
      setState((s) => ({ ...s, pending: s.pending + items.length }));
      try {
        await outbox.enqueueAll(items);
      } catch {
        setState((s) => ({ ...s, pending: Math.max(0, s.pending - items.length) }));
        return false;
      }
      // 裏で送る。件数の読み直しが失敗しても、積んだ記録には関係しない。
      void flush().catch(() => {});
      return true;
    },
    [outbox, flush],
  );

  const enqueue = useCallback((item: QueueItem) => enqueueAll([item]), [enqueueAll]);

  const clearRejected = useCallback(async () => {
    await outbox.clearRejected();
    await refresh();
  }, [outbox, refresh]);

  // 旧版が localStorage に残したぶんを引き取ってから数える。
  useEffect(() => {
    void adoptLegacyQueue()
      .catch(() => {})
      .then(refresh);
  }, [refresh]);

  return { ...state, enqueue, enqueueAll, flush, refresh, clearRejected };
}
