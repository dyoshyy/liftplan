import { useCallback, useState } from 'react';
import { getJSON, Unauthorized } from '../api/client';
import type {
  Day,
  Exercise,
  ExercisesResponse,
  HistoryResponse,
  LastPerformance,
  Program,
  RecordedSet,
  Session,
  StatsResponse,
} from '../api/types';
import { addDays, today } from '../domain/date';
import { acceptedDeload } from '../storage/local';

export type Data = {
  session: Session | null;
  exercises: Exercise[];
  names: Map<string, string>;
  last: Record<string, LastPerformance>;
  days: Day[];
  stats: StatsResponse;
  program: Program | null;
  /** doneToday は今日の実績。サーバーから復元し、記録のたびに手元でも進める。 */
  doneToday: Map<string, RecordedSet[]>;
};

const empty: Data = {
  session: null,
  exercises: [],
  names: new Map(),
  last: {},
  days: [],
  stats: { from: '', to: '', trends: [], weekly_volume: [] },
  program: null,
  doneToday: new Map(),
};

export type LoadState = 'loading' | 'ready' | 'offline' | 'unauthorized';

export function useLiftplan() {
  const [data, setData] = useState<Data>(empty);
  const [status, setStatus] = useState<LoadState>('loading');

  // loadToday は今日のメニューだけを取り直す。
  //
  // 未来のメニューを手元に持たない。表示は常にサーバーから取り直す。
  // 持つと、記録した結果が反映されているのか分からなくなる。
  const loadToday = useCallback(async (deloadAccepted?: string[]) => {
    const date = today();
    // 引数が無いときは、その日に承認したものを使う。更新やリロードでも
    // 承認が効いたままになる。
    const accepted = deloadAccepted ?? acceptedDeload(date);
    const q = accepted.length ? `&deload_accepted=${encodeURIComponent(accepted.join(','))}` : '';
    const session = await getJSON<Session>(`/api/sessions?date=${date}${q}`);
    setData((d) => ({ ...d, session }));
  }, []);

  const loadAll = useCallback(async () => {
    const date = today();
    try {
      const [exercises, history, stats, program] = await Promise.all([
        getJSON<ExercisesResponse>('/api/exercises'),
        getJSON<HistoryResponse>(`/api/set-logs?from=${addDays(date, -56)}&to=${date}`),
        getJSON<StatsResponse>(`/api/stats?from=${addDays(date, -180)}&to=${date}`),
        getJSON<Program>('/api/program'),
      ]);

      // 「どこまでやったか」はサーバーの実績から復元する。端末の中だけに
      // 持つと、画面を閉じた瞬間に分からなくなる。
      const doneToday = new Map<string, RecordedSet[]>();
      const t = history.days.find((d) => d.date === date);
      for (const e of t?.exercises ?? []) doneToday.set(e.exercise_id, e.sets);

      setData({
        session: null,
        exercises: exercises.exercises,
        names: new Map(exercises.exercises.map((e) => [e.id, e.name])),
        last: history.last_performances,
        days: history.days,
        stats,
        program,
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
        const next = replacing
          ? list.map((r) => (r.id === replacing ? set : r))
          : [...list, set];
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
