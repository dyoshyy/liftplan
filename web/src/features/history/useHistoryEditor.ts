import { useCallback, useState } from 'react';
import type { RecordedSet } from '../../api/types';
import type { DayChange } from '../../domain/days';
import type { SetValues } from '../../domain/sets';
import type { QueueItem } from '../../outbox/db';
import { planRecord, planUndo } from '../today/useRecordOrchestrator';

/** EditTarget は履歴で直そうとしている1セット。 */
export type EditTarget = {
  date: string;
  exerciseId: string;
  name: string;
  /** index はその種目の何セット目か（0始まり）。見出しに出すだけ。 */
  index: number;
  set: RecordedSet;
};

export type EditPlan = {
  /** queue は待ち行列に積むもの。**この順で積む。** */
  queue: QueueItem[];
  /** change は手元の記録（直近の記録と取ってある月）に当てる変更。 */
  change: DayChange;
};

// 履歴での修正と削除。
//
// **何をどの順で積むかは今日の画面と同じ planRecord / planUndo に任せる。**
// 「消してから同じIDで入れ直す」を2箇所に書くと、片方だけ直したときに
// 記録が消える。違うのは日付だけで、今日ではなく**その記録の日付**を渡す。
// 今日の日付で入れ直すと、8月の記録が今日へ移る。

export function planHistoryEdit(target: EditTarget, values: SetValues): EditPlan {
  const plan = planRecord({
    plan: { exercise_id: target.exerciseId },
    recorded: target.set,
    values,
    date: target.date,
    // 修正は同じIDを使い回すので採番しない。呼ばれたら planRecord の前提が崩れている。
    newId: () => target.set.id,
  });
  return {
    queue: plan.queue,
    change: { kind: 'put', date: target.date, exerciseId: target.exerciseId, name: target.name, set: plan.local.set },
  };
}

export function planHistoryDelete(target: EditTarget): EditPlan {
  const plan = planUndo({ plan: { exercise_id: target.exerciseId }, recorded: target.set });
  return {
    queue: plan.queue,
    change: { kind: 'remove', date: target.date, exerciseId: target.exerciseId, id: target.set.id },
  };
}

type Deps = {
  enqueue: (item: QueueItem) => Promise<void>;
  /** onApplied は手元の記録に変更を当てる。直近の記録と取ってある月の両方。 */
  onApplied: (change: DayChange) => void;
};

// 編集シートの開閉と、修正・削除の手順を束ねる。判断は上の2つにあり、
// ここは返ってきた順に実行するだけ。
//
// **推定1RMや今日のメニューは取り直さない。**トレーニングの最中にメニューが
// 動くと困る。次に読み込み直したときに反映される。
export function useHistoryEditor({ enqueue, onApplied }: Deps) {
  const [target, setTarget] = useState<EditTarget | null>(null);

  const open = useCallback((t: EditTarget) => setTarget(t), []);
  const close = useCallback(() => setTarget(null), []);

  const run = useCallback(
    async (plan: EditPlan) => {
      for (const item of plan.queue) await enqueue(item);
      onApplied(plan.change);
      setTarget(null);
    },
    [enqueue, onApplied],
  );

  const save = useCallback(
    async (values: SetValues) => {
      if (target) await run(planHistoryEdit(target, values));
    },
    [target, run],
  );

  const remove = useCallback(async () => {
    if (target) await run(planHistoryDelete(target));
  }, [target, run]);

  return { target, open, close, save, remove };
}

export type HistoryEditor = ReturnType<typeof useHistoryEditor>;
