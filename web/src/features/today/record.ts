import type { RecordedSet } from '../../api/types';
import type { QueueItem } from '../../outbox/db';

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
