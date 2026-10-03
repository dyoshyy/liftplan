import { useCallback, useState } from 'react';
import type { Day, RecordedSet } from '../../api/types';
import { today } from '../../domain/date';
import { newId } from '../../domain/id';
import type { QueueItem } from '../../outbox/db';
import { judgePersonalRecord, previousSets, type PersonalRecord } from './pr';
import { createClaims } from './claims';
import type { SheetTarget } from './RecordSheet';

type Deps = {
  /** enqueueAll は端末に積んだところで返る（送信の完了は待たない）。積めなければ false。 */
  enqueueAll: (items: readonly QueueItem[]) => Promise<boolean>;
  /** onSaveFailed は端末に積めなかったことを知らせる。手元の記録は元に戻したあとで呼ぶ。 */
  onSaveFailed: (message: string) => void;
  onRecordLocally: (exerciseId: string, set: RecordedSet, replacing?: string) => void;
  onForgetLocally: (exerciseId: string, id: string) => void;
  onRestStart: () => void;
  /** history は自己ベストの判定に使う母集団。どう畳むかは pr.ts が決める。 */
  history: {
    days: readonly Day[];
    doneToday: ReadonlyMap<string, readonly RecordedSet[]>;
    /** loadOffsets は種目ごとの、加重に足すと体重込みの負荷になる量（自重種目だけ）。 */
    loadOffsets: Readonly<Record<string, number>>;
  };
  nameOf: (exerciseId: string) => string;
  /** onPersonalRecord は更新だったときだけ呼ぶ。 */
  onPersonalRecord: (pr: PersonalRecord) => void;
};

// 記録の手順を決める。副作用は持たない。
//
// **「何をどの順で積むか」をここに置くのが要点。**以前はこの判断が
// Today.tsx の中にあり、DOM を立てないと検査できなかった。順序を1つ
// 間違えると記録が消えるのに、守っているものが何も無かった。

type Plan = { exercise_id: string };
type Values = { weight: number; reps: number; rir: number };

export type RecordInput = {
  plan: Plan;
  /** recorded は直しているセット。新規なら undefined。 */
  recorded: RecordedSet | undefined;
  values: Values;
  date: string;
  /** newId は採番。差し替えられるようにして、検査から時刻とランダムを外す。 */
  newId: () => string;
};

export type RecordPlan = {
  /** queue は待ち行列に積むもの。**この順で積む。** */
  queue: QueueItem[];
  /** local は手元の実績をどう進めるか。送信の完了を待たずに反映する。 */
  local: { exerciseId: string; set: RecordedSet; replacing: string | undefined };
  /** startRest は休憩を始めるか。 */
  startRest: boolean;
};

const setLogPath = (id: string) => `/api/set-logs/${encodeURIComponent(id)}`;

export function planRecord({ plan, recorded, values, date, newId }: RecordInput): RecordPlan {
  const queue: QueueItem[] = [];

  // 直す場合は、古い記録を消してから入れ直す。同じIDで内容を変えると
  // サーバーが衝突として弾く（そういう契約にしてある）。
  if (recorded) {
    queue.push({ path: setLogPath(recorded.id), method: 'DELETE' });
  }

  // 直すときは同じIDを使い回す。新しいIDにすると、並び順が id 順
  // （＝作った時刻順）なので、直したセットだけが末尾に飛ぶ。
  // 1セット目を直したら3セット目になって出てくる。
  // 先に消してから入れ直すので、同一IDでも衝突にはならない。
  const id = recorded ? recorded.id : newId();
  const set: RecordedSet = { id, weight_kg: values.weight, reps: values.reps, rir: values.rir };

  queue.push({
    path: '/api/set-logs',
    body: { logs: [{ id, date, exercise_id: plan.exercise_id, ...toLog(values) }] },
  });

  return {
    queue,
    local: { exerciseId: plan.exercise_id, set, replacing: recorded?.id },
    // 過去のセットを直しただけでタイマーが走ると、いま休んでいる時間が
    // 上書きされる。
    startRest: recorded === undefined,
  };
}

/**
 * Rollback は保存できなかったとき、手元の記録を元に戻す指示。
 *
 * 記録を押した瞬間に手元の記録を進める（楽観的更新）ので、端末に積めなかった
 * ときは「実際に保存されている状態」へ戻す。戻さないと、画面には記録済みなのに
 * 何も保存されていない、という食い違いが残る。
 */
export type Rollback =
  /** forget は新しく足したセットを取り除く。 */
  | { kind: 'forget'; exerciseId: string; id: string }
  /** restore は元の値に戻す。修正と取り消しの巻き戻し。 */
  | { kind: 'restore'; exerciseId: string; set: RecordedSet };

/**
 * rollbackRecord は記録（新規・修正）が保存できなかったときの戻し方。
 *
 * 修正を取り除くと、元からあった記録まで画面から消える。保存されているのは
 * 修正前の値なので、それに戻す。
 */
export function rollbackRecord(plan: RecordPlan, previous: RecordedSet | undefined): Rollback {
  return previous
    ? { kind: 'restore', exerciseId: plan.local.exerciseId, set: previous }
    : { kind: 'forget', exerciseId: plan.local.exerciseId, id: plan.local.set.id };
}

/** rollbackUndo は取り消しが保存できなかったときの戻し方。記録は残ったままなので、画面にも戻す。 */
export function rollbackUndo(plan: UndoPlan, removed: RecordedSet): Rollback {
  return { kind: 'restore', exerciseId: plan.forget.exerciseId, set: removed };
}

/**
 * saveFailureMessage は保存できなかったことを、何の記録かで言う。
 * 「保存できませんでした」だけでは、どれを入れ直せばよいか分からない。
 */
export function saveFailureMessage(kind: 'record' | 'undo', name: string, set: RecordedSet): string {
  const what = `${name} ${set.weight_kg}kg × ${set.reps}`;
  return kind === 'undo' ? `${what} の取り消し` : what;
}

export type UndoPlan = {
  queue: QueueItem[];
  forget: { exerciseId: string; id: string };
};

export function planUndo({ plan, recorded }: { plan: Plan; recorded: RecordedSet }): UndoPlan {
  return {
    queue: [{ path: setLogPath(recorded.id), method: 'DELETE' }],
    forget: { exerciseId: plan.exercise_id, id: recorded.id },
  };
}

const toLog = (v: Values) => ({ weight_kg: v.weight, reps: v.reps, rir: v.rir });

// 記録シートの開閉と、記録・取り消しの手順を束ねる。
//
// **判断は record.ts に置き、ここは順に実行するだけ。**フックの中で
// 分岐を書くと、また DOM を立てないと検査できないものに戻る。
//
// **押した瞬間に画面を進める（楽観的更新）。**手元の記録・休憩・自己ベストの
// 演出・シートを閉じるまでを同期で済ませ、端末への保存は裏で行う。以前は
// 保存を待ってから進めていたので、押してから反応するまでが保存の長さだけ
// かかった。保存できなかったときだけ、手元の記録を元に戻して知らせる。
//
// 連打は2段で止める。
//   1. 操作のID（draftId）はシートを開いたときに1回だけ振る。2回通っても
//      同じID・同じ内容なので、サーバーも手元の記録も2回目を吸収する（冪等）
//   2. 休憩の開始や演出のような冪等でない副作用は、claims で1回だけ通す
//
// 待ち行列に積む順序は `plan.queue` の順そのまま。ここで並べ替えない。
export function useRecordOrchestrator({
  enqueueAll,
  onSaveFailed,
  onRecordLocally,
  onForgetLocally,
  onRestStart,
  history,
  nameOf,
  onPersonalRecord,
}: Deps) {
  const [sheet, setSheet] = useState<SheetTarget | null>(null);
  const [claims] = useState(createClaims);

  const open = useCallback(
    (target: Omit<SheetTarget, 'draftId'>) => setSheet({ ...target, draftId: newId() }),
    [],
  );
  const close = useCallback(() => setSheet(null), []);

  // 保存できなかったとき、手元の記録を元に戻して知らせる。
  const undoLocally = useCallback(
    (rollback: Rollback) => {
      if (rollback.kind === 'forget') onForgetLocally(rollback.exerciseId, rollback.id);
      else onRecordLocally(rollback.exerciseId, rollback.set);
    },
    [onForgetLocally, onRecordLocally],
  );

  // 端末への保存。画面はもう進めてあるので、待たずに裏で回す。
  const persist = useCallback(
    async (queue: readonly QueueItem[], rollback: Rollback, message: string) => {
      if (await enqueueAll(queue)) return;
      undoLocally(rollback);
      onSaveFailed(message);
    },
    [enqueueAll, undoLocally, onSaveFailed],
  );

  const record = useCallback(
    (values: { weight: number; reps: number; rir: number }) => {
      if (!sheet) return;
      if (!claims.claim(`${sheet.draftId}:record`)) return;

      const date = today();
      const plan = planRecord({
        plan: sheet.plan,
        recorded: sheet.recorded,
        values,
        date,
        newId: () => sheet.draftId,
      });

      // 自己ベストの判定は**手元の記録を進める前**にやる。進めたあとに
      // 母集団を作ると、いま記録したセット自身が「過去の最高」に入り、
      // どんな更新も自分自身を超えられなくなる。
      const pr = judgePersonalRecord({
        exerciseId: sheet.plan.exercise_id,
        name: nameOf(sheet.plan.exercise_id),
        values,
        loadOffsetKg: history.loadOffsets[sheet.plan.exercise_id] ?? 0,
        previous: previousSets({
          days: history.days,
          doneToday: history.doneToday,
          exerciseId: sheet.plan.exercise_id,
          today: date,
          excludeId: sheet.recorded?.id,
        }),
      });

      onRecordLocally(plan.local.exerciseId, plan.local.set, plan.local.replacing);
      if (plan.startRest) onRestStart();
      if (pr) onPersonalRecord(pr);
      setSheet(null);

      void persist(
        plan.queue,
        rollbackRecord(plan, sheet.recorded),
        saveFailureMessage('record', nameOf(plan.local.exerciseId), plan.local.set),
      );
    },
    [claims, sheet, onRecordLocally, onRestStart, history, nameOf, onPersonalRecord, persist],
  );

  const undo = useCallback(() => {
    if (!sheet?.recorded) return;
    if (!claims.claim(`${sheet.draftId}:undo`)) return;

    const plan = planUndo({ plan: sheet.plan, recorded: sheet.recorded });

    onForgetLocally(plan.forget.exerciseId, plan.forget.id);
    setSheet(null);

    void persist(
      plan.queue,
      rollbackUndo(plan, sheet.recorded),
      saveFailureMessage('undo', nameOf(plan.forget.exerciseId), sheet.recorded),
    );
  }, [claims, sheet, onForgetLocally, nameOf, persist]);

  return { sheet, open, close, record, undo };
}
