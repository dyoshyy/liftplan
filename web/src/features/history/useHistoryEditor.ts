import { useCallback, useState } from 'react';
import type { ExerciseLog, RecordedSet } from '../../api/types';
import type { DayChange } from '../../domain/days';
import { newId } from '../../domain/id';
import type { SetValues } from '../../domain/sets';
import type { Enqueue, QueueItem } from '../../outbox/db';
import { planRecord, planUndo } from '../today/useRecordOrchestrator';

/** EditTarget は履歴で直そうとしている1セット。 */
export type EditTarget = {
  kind: 'edit';
  date: string;
  exerciseId: string;
  name: string;
  /** index はその種目の何セット目か（0始まり）。見出しに出すだけ。 */
  index: number;
  set: RecordedSet;
};

/** AddTarget は履歴で足そうとしている1セット。まだ ID は無い（足すときに採番する）。 */
export type AddTarget = {
  kind: 'add';
  date: string;
  exerciseId: string;
  name: string;
  /** index は足すと何セット目になるか（0始まり）。見出しに出すだけ。 */
  index: number;
  /** initial は入力の初期値。null なら空で開く。 */
  initial: Pick<RecordedSet, 'weight_kg' | 'reps' | 'rir'> | null;
};

/** SheetTarget は編集シートが開いている相手。直すのか足すのかで、積むものが違う。 */
export type SheetTarget = EditTarget | AddTarget;

export type EditPlan = {
  /** queue は待ち行列に積むもの。**この順で積む。** */
  queue: QueueItem[];
  /** change は手元の記録（直近の記録と取ってある月）に当てる変更。 */
  change: DayChange;
};

// 履歴での修正・追加・削除。
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
    change: {
      kind: 'put',
      date: target.date,
      exerciseId: target.exerciseId,
      name: target.name,
      set: plan.local.set,
    },
  };
}

export function planHistoryDelete(target: EditTarget): EditPlan {
  const plan = planUndo({ plan: { exercise_id: target.exerciseId }, recorded: target.set });
  return {
    queue: plan.queue,
    change: { kind: 'remove', date: target.date, exerciseId: target.exerciseId, id: target.set.id },
  };
}

/**
 * planHistoryAdd は過去の日にセットを足す。
 *
 * 今日の記録と同じ planRecord を、直す相手なし（recorded: undefined）で呼ぶ。
 * 違うのは日付で、今日ではなく足す日を渡す。startRest は使わない。
 * 過去の日に足しただけで、いま休んでいる時間を上書きしない。
 */
export function planHistoryAdd(
  target: { date: string; exerciseId: string; name: string },
  values: SetValues,
  newId: () => string,
): EditPlan {
  const plan = planRecord({
    plan: { exercise_id: target.exerciseId },
    recorded: undefined,
    values,
    date: target.date,
    newId,
  });
  return {
    queue: plan.queue,
    change: {
      kind: 'put',
      date: target.date,
      exerciseId: target.exerciseId,
      name: target.name,
      set: plan.local.set,
    },
  };
}

/** addSetTarget はその日のその種目に、次のセットを足す相手。入力は最後のセットの値で始める。 */
export function addSetTarget(date: string, log: ExerciseLog): AddTarget {
  return {
    kind: 'add',
    date,
    exerciseId: log.exercise_id,
    name: log.name || log.exercise_id,
    index: log.sets.length,
    initial: log.sets[log.sets.length - 1] ?? null,
  };
}

type Deps = {
  enqueue: Enqueue;
  /** onApplied は手元の記録に変更を当てる。直近の記録と取ってある月の両方。 */
  onApplied: (change: DayChange) => void;
};

// 編集シートの開閉と、修正・追加・削除の手順を束ねる。判断は上の2つにあり、
// ここは返ってきた順に実行するだけ。
//
// **推定1RMや今日のメニューは取り直さない。**トレーニングの最中にメニューが
// 動くと困る。次に読み込み直したときに反映される。
export function useHistoryEditor({ enqueue, onApplied }: Deps) {
  const [target, setTarget] = useState<SheetTarget | null>(null);

  const open = useCallback((t: EditTarget) => setTarget(t), []);
  const openAdd = useCallback((t: AddTarget) => setTarget(t), []);
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
      if (target?.kind === 'edit') await run(planHistoryEdit(target, values));
    },
    [target, run],
  );

  const add = useCallback(
    async (values: SetValues) => {
      if (target?.kind === 'add') await run(planHistoryAdd(target, values, newId));
    },
    [target, run],
  );

  const remove = useCallback(async () => {
    if (target?.kind === 'edit') await run(planHistoryDelete(target));
  }, [target, run]);

  return { target, open, openAdd, close, save, add, remove };
}

export type HistoryEditor = ReturnType<typeof useHistoryEditor>;
