import { useCallback, useState } from 'react';
import type { RecordedSet } from '../../api/types';
import { today } from '../../domain/date';
import { newId } from '../../domain/id';
import type { QueueItem } from '../../outbox/db';
import { planRecord, planUndo } from './record';
import type { SheetTarget } from './RecordSheet';

type Deps = {
  enqueue: (item: QueueItem) => Promise<void>;
  onRecordLocally: (exerciseId: string, set: RecordedSet, replacing?: string) => void;
  onForgetLocally: (exerciseId: string, id: string) => void;
  onRestStart: () => void;
};

// 記録シートの開閉と、記録・取り消しの手順を束ねる。
//
// **判断は record.ts に置き、ここは順に実行するだけ。**フックの中で
// 分岐を書くと、また DOM を立てないと検査できないものに戻る。
//
// 待ち行列に積む順序は `plan.queue` の順そのまま。ここで並べ替えない。
export function useRecordOrchestrator({
  enqueue,
  onRecordLocally,
  onForgetLocally,
  onRestStart,
}: Deps) {
  const [sheet, setSheet] = useState<SheetTarget | null>(null);

  const open = useCallback((target: SheetTarget) => setSheet(target), []);
  const close = useCallback(() => setSheet(null), []);

  const record = useCallback(
    async (values: { weight: number; reps: number; rir: number }) => {
      if (!sheet) return;

      const plan = planRecord({
        plan: sheet.plan,
        recorded: sheet.recorded,
        values,
        date: today(),
        newId,
      });

      for (const item of plan.queue) await enqueue(item);

      onRecordLocally(plan.local.exerciseId, plan.local.set, plan.local.replacing);
      if (plan.startRest) onRestStart();
      setSheet(null);
    },
    [sheet, enqueue, onRecordLocally, onRestStart],
  );

  const undo = useCallback(async () => {
    if (!sheet?.recorded) return;

    const plan = planUndo({ plan: sheet.plan, recorded: sheet.recorded });
    for (const item of plan.queue) await enqueue(item);

    onForgetLocally(plan.forget.exerciseId, plan.forget.id);
    setSheet(null);
  }, [sheet, enqueue, onForgetLocally]);

  return { sheet, open, close, record, undo };
}
