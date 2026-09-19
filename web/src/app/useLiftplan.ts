import { useCallback, useState } from 'react';
import { getJSON, Unauthorized } from '../api/client';
import type {
  Day,
  Exercise,
  ExercisesResponse,
  HistoryResponse,
  LastPerformance,
  RecordedSet,
  Session,
} from '../api/types';
import { addDays, today } from '../domain/date';

/** LoadResult は読み込みの結果。状態にはしない（呼び手が決める）。 */
export type LoadResult = { ok: true } | { ok: false; reason: 'offline' | 'unauthorized' };

export type Data = {
  session: Session | null;
  names: Map<string, string>;
  /** exercises は種目マスタ。設定画面が部位ごとにまとめるのに使う。 */
  exercises: Exercise[];
  last: Record<string, LastPerformance>;
  /** doneToday は今日の実績。サーバーから復元し、記録のたびに手元でも進める。 */
  doneToday: Map<string, RecordedSet[]>;
  /**
   * days は日ごとの記録。履歴の画面が使う。
   *
   * `/api/set-logs` の応答は「今日どこまでやったか」の復元に必ず要るので、
   * 既に取ってある。捨てずに持つだけで、往復は1本も増えない。
   */
  days: Day[];
};

const empty: Data = {
  session: null,
  names: new Map(),
  exercises: [],
  last: {},
  doneToday: new Map(),
  days: [],
};

// ここで読むのは2つだけ。
//
// /api/stats（履歴）と /api/program（設定）はここでは叩かない。それぞれの
// 画面が開かれたときに自分で取りに行く（D-127）。ジムで毎回開く「今日」の
// 読み込みに、見ていない画面の往復を混ぜない。
export function useLiftplan() {
  const [data, setData] = useState<Data>(empty);

  // loadToday は今日のメニューだけを取り直す。
  //
  // 未来のメニューを手元に持たない。表示は常にサーバーから取り直す。
  // 持つと、記録した結果が反映されているのか分からなくなる。
  const loadToday = useCallback(async () => {
    const session = await getJSON<Session>(`/api/sessions?date=${today()}`);
    setData((d) => ({ ...d, session }));
  }, []);

  const loadAll = useCallback(async (): Promise<LoadResult> => {
    const date = today();
    try {
      const [exercises, history] = await Promise.all([
        getJSON<ExercisesResponse>('/api/exercises'),
        getJSON<HistoryResponse>(`/api/set-logs?from=${addDays(date, -56)}&to=${date}`),
      ]);

      // 「どこまでやったか」はサーバーの実績から復元する。端末の中だけに
      // 持つと、画面を閉じた瞬間に分からなくなる。
      const doneToday = new Map<string, RecordedSet[]>();
      const t = history.days.find((d) => d.date === date);
      for (const e of t?.exercises ?? []) doneToday.set(e.exercise_id, e.sets);

      setData({
        session: null,
        names: new Map(exercises.exercises.map((e: Exercise) => [e.id, e.name])),
        exercises: exercises.exercises,
        last: history.last_performances,
        doneToday,
        // サーバーが新しい日から順に返す（query.History.Days）。並べ替えない。
        days: history.days,
      });

      await loadToday();
      return { ok: true } as const;
    } catch (e) {
      // 結果を返すだけで、状態は持たない。**どう扱うかは呼び手が決める。**
      // ここで状態を持つと、セッションの遷移が2箇所に割れる。
      return { ok: false, reason: e instanceof Unauthorized ? 'unauthorized' : 'offline' } as const;
    }
  }, [loadToday]);

  // recordLocally は手元の実績を先に進める。
  //
  // 送信の完了を待つと、電波が悪いときに「押したのに反応しない」画面になる。
  const recordLocally = useCallback(
    (exerciseId: string, set: RecordedSet, replacing?: string) => {
      setData((d) => {
        const list = d.doneToday.get(exerciseId) ?? [];
        const next = replacing ? list.map((r) => (r.id === replacing ? set : r)) : [...list, set];
        return { ...d, doneToday: new Map(d.doneToday).set(exerciseId, next) };
      });
    },
    [],
  );

  const forgetLocally = useCallback((exerciseId: string, id: string) => {
    setData((d) => {
      const next = (d.doneToday.get(exerciseId) ?? []).filter((r) => r.id !== id);
      return { ...d, doneToday: new Map(d.doneToday).set(exerciseId, next) };
    });
  }, []);

  return { data, loadAll, loadToday, recordLocally, forgetLocally };
}
