import { useCallback, useEffect, useState } from 'react';
import { getJSON } from '../../api/client';
import { newId } from '../../domain/id';
import type { Enqueue } from '../../outbox/db';
import { planRun, planUndoRun, visibleRuns, type Run, type RunValues } from './run';

type Options = {
  enqueue: Enqueue;
  /** onSaveFailed は端末に積めなかったことを知らせる。画面の表示は元に戻したあとで呼ぶ。 */
  onSaveFailed: (message: string) => void;
  /** date はランを付ける日（端末の今日）。 */
  date: string;
};

// useRunRecord はランの記録の手順を持つ。何を積むかは run.ts が決め、ここは
// 返ってきたものを積むだけ。
//
// 今日すでに記録したランはサーバーから読む。セットと違い、ランは開き直して
// 記録し直すと別の ID の2件目になる。記録済みが見えていないと二重に入る。
// 読めなかったら（圏外）この画面で記録したものだけを出す。
export function useRunRecord({ enqueue, onSaveFailed, date }: Options) {
  const [server, setServer] = useState<Run[]>([]);
  const [added, setAdded] = useState<Run[]>([]);
  const [removed, setRemoved] = useState<ReadonlySet<string>>(new Set());
  // draftId はシートを開いた1回の操作の ID。開いている間だけある。
  const [draftId, setDraftId] = useState<string | null>(null);

  useEffect(() => {
    let live = true;
    getJSON<{ runs: Run[] }>(`/api/run-logs?from=${date}&to=${date}`)
      .then((res) => live && setServer(res.runs))
      .catch(() => undefined);
    return () => {
      live = false;
    };
  }, [date]);

  const open = useCallback(() => setDraftId(newId()), []);
  const close = useCallback(() => setDraftId(null), []);

  const record = useCallback(
    async (values: RunValues) => {
      if (draftId === null) return;
      const run: Run = { id: draftId, date, ...values };
      setAdded((a) => [...a, run]);
      setDraftId(null);
      if (await enqueue(planRun({ id: run.id, date, values }))) return;
      setAdded((a) => a.filter((r) => r.id !== run.id));
      onSaveFailed('ランの記録を端末に保存できませんでした');
    },
    [draftId, date, enqueue, onSaveFailed],
  );

  const undo = useCallback(
    async (id: string) => {
      setRemoved((s) => new Set(s).add(id));
      if (await enqueue(planUndoRun(id))) return;
      setRemoved((s) => {
        const next = new Set(s);
        next.delete(id);
        return next;
      });
      onSaveFailed('ランの取り消しを端末に保存できませんでした');
    },
    [enqueue, onSaveFailed],
  );

  return {
    runs: visibleRuns(server, added, removed),
    sheetOpen: draftId !== null,
    open,
    close,
    record,
    undo,
  };
}
