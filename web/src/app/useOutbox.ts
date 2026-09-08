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

  const enqueue = useCallback(
    async (item: QueueItem) => {
      // 楽観的に数えておく。押した直後に「未送信」と出ないと、
      // 記録できたのか分からない。
      setState((s) => ({ ...s, pending: s.pending + 1 }));
      try {
        await outbox.enqueue(item);
      } catch {
        setState((s) => ({ ...s, pending: Math.max(0, s.pending - 1) }));
        return;
      }
      await flush();
    },
    [outbox, flush],
  );

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

  return { ...state, enqueue, flush, refresh, clearRejected };
}
