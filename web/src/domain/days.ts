import type { Day, ExerciseLog, RecordedSet } from '../api/types';

/** DayChange は手元の記録に1セット分の変更を当てる指示。 */
export type DayChange =
  /** put は同じIDがあれば置き換え、無ければ足す。記録も修正もこれ1つ。 */
  | { kind: 'put'; date: string; exerciseId: string; name: string; set: RecordedSet }
  | { kind: 'remove'; date: string; exerciseId: string; id: string };

const withTotal = (date: string, exercises: ExerciseLog[]): Day => ({
  date,
  exercises,
  total_sets: exercises.reduce((a, e) => a + e.sets.length, 0),
});

function putSet(ex: ExerciseLog, set: RecordedSet): ExerciseLog {
  const found = ex.sets.some((s) => s.id === set.id);
  return {
    ...ex,
    sets: found ? ex.sets.map((s) => (s.id === set.id ? set : s)) : [...ex.sets, set],
  };
}

/**
 * patchDays は日ごとの記録（新しい順）に変更を当てた新しい配列を返す。
 *
 * 送信の完了を待たずに画面を進めるためにある。今日の記録と、履歴での
 * 修正・削除の両方がこれを通る。**経路を1本にしてある**のは、今日の画面と
 * 履歴で「どこに差し込むか」「空になったら消すか」が食い違うと、同じ記録が
 * 画面によって違って見えるため。
 *
 * 置き換えは同じIDの位置で行う。修正は同じIDで入れ直す（planRecord）ので、
 * 末尾に足すと直したセットだけが後ろへ飛ぶ。
 *
 * 空になった種目と日は消す。残すと「0種目 0セット」のカードが出て、
 * 月の回数にも数えられる。
 *
 * 元の配列は書き換えない。
 */
export function patchDays(days: readonly Day[], change: DayChange): Day[] {
  const i = days.findIndex((d) => d.date === change.date);

  if (change.kind === 'remove') {
    const day = days[i];
    if (!day) return [...days];
    const exercises = day.exercises
      .map((e) =>
        e.exercise_id === change.exerciseId ? { ...e, sets: e.sets.filter((s) => s.id !== change.id) } : e,
      )
      .filter((e) => e.sets.length > 0);
    const next = withTotal(day.date, exercises);
    return exercises.length === 0 ? days.filter((_, j) => j !== i) : days.map((d, j) => (j === i ? next : d));
  }

  const day = days[i];
  if (!day) {
    const fresh = withTotal(change.date, [
      { exercise_id: change.exerciseId, name: change.name, sets: [change.set] },
    ]);
    // 新しい順を保つ。日付は YYYY-MM-DD なので文字列で比べられる。
    const at = days.findIndex((d) => d.date < change.date);
    return at === -1 ? [...days, fresh] : [...days.slice(0, at), fresh, ...days.slice(at)];
  }

  const has = day.exercises.some((e) => e.exercise_id === change.exerciseId);
  const exercises = has
    ? day.exercises.map((e) => (e.exercise_id === change.exerciseId ? putSet(e, change.set) : e))
    : [...day.exercises, { exercise_id: change.exerciseId, name: change.name, sets: [change.set] }];
  return days.map((d, j) => (j === i ? withTotal(d.date, exercises) : d));
}

type LocalRecords = {
  days: Day[];
  doneToday: Map<string, RecordedSet[]>;
};

/**
 * applyDayChange は手元の記録（日ごとの記録と今日の実績）に変更を当てる。
 *
 * 今日の実績は、当てたあとの日ごとの記録から作り直す。2つを別々に進めて
 * いたせいで、今日記録した分が開き直すまで履歴に出なかった。
 *
 * 過去の日の変更では今日の実績に触らない（同じ Map をそのまま返す）。
 */
export function applyDayChange(state: LocalRecords, change: DayChange, today: string): LocalRecords {
  const days = patchDays(state.days, change);
  if (change.date !== today) return { days, doneToday: state.doneToday };

  const sets =
    days.find((d) => d.date === today)?.exercises.find((e) => e.exercise_id === change.exerciseId)?.sets ??
    [];
  return { days, doneToday: new Map(state.doneToday).set(change.exerciseId, sets) };
}
