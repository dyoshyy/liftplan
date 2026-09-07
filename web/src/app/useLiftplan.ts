import { useCallback, useState } from 'react';
import { getJSON, Unauthorized } from '../api/client';
import type {
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
};

const empty: Data = {
  session: null,
  names: new Map(),
  last: {},
  doneToday: new Map(),
};

export type LoadState = 'loading' | 'ready' | 'offline' | 'unauthorized';

// 読むのは2つだけ。
//
// /api/stats（履歴）と /api/program（設定）は叩かない。画面を落としたので
// 読む相手がいない。サーバー側は残してあるので、戻すときは呼び出しを
// 足すだけで済む。
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
