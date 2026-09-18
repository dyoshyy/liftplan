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

export type Data = {
  session: Session | null;
  names: Map<string, string>;
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
  last: {},
  doneToday: new Map(),
  days: [],
};

export type LoadState = 'loading' | 'ready' | 'offline' | 'unauthorized';

// ここで読むのは2つだけ。
//
// /api/stats（履歴）と /api/program（設定）はここでは叩かない。それぞれの
// 画面が開かれたときに自分で取りに行く（D-128）。ジムで毎回開く「今日」の
// 読み込みに、見ていない画面の往復を混ぜない。
export function useLiftplan() {
  const [data, setData] = useState<Data>(empty);
  const [status, setStatus] = useState<LoadState>('loading');

  // loadToday は今日のメニューだけを取り直す。
  //
  // 未来のメニューを手元に持たない。表示は常にサーバーから取り直す。
  // 持つと、記録した結果が反映されているのか分からなくなる。
  const loadToday = useCallback(async () => {
    const session = await getJSON<Session>(`/api/sessions?date=${today()}`);
    setData((d) => ({ ...d, session }));
  }, []);

  const loadAll = useCallback(async () => {
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
        last: history.last_performances,
        doneToday,
        // サーバーが新しい日から順に返す（query.History.Days）。並べ替えない。
        days: history.days,
      });

      await loadToday();
      setStatus('ready');
    } catch (e) {
      setStatus(e instanceof Unauthorized ? 'unauthorized' : 'offline');
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

  return { data, status, setStatus, loadAll, loadToday, recordLocally, forgetLocally };
}
