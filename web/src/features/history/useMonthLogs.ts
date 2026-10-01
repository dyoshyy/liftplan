import { useCallback, useEffect, useState } from 'react';
import { getJSON } from '../../api/client';
import type { Day, HistoryResponse } from '../../api/types';
import { today } from '../../domain/date';
import type { DayChange } from '../../domain/days';
import {
  canGoNext,
  monthOf,
  patchFetched,
  pickMonthDays,
  planMonthLoad,
  shiftMonth,
  type Month,
} from './month';

export type MonthLogs = {
  month: Month;
  /** その月の日（新しい順）。まだ読めていなければ null。 */
  days: Day[] | null;
  /** 読めなかったときの一言。空なら出さない。 */
  error: string;
  canNext: boolean;
  prev: () => void;
  next: () => void;
  reload: () => void;
  /** patch は取ってある月に変更を当てる。今月は直近の記録の側で進む。 */
  patch: (change: DayChange) => void;
};

/**
 * useMonthLogs は履歴を月ごとに見せるための記録を用意する。
 *
 * どこから持ってくるかは `planMonthLoad`、何を描くかは `pickMonthDays` が
 * 決める。ここは決まったとおりに取りに行って、取ってきた月を持っておくだけ。
 *
 * **取ってきた応答は `.days` しか使わない。**同じ応答に入っている
 * `last_performances` は `to` を基準にした「前回」なので、過去の月の分を
 * `useLiftplan` に書くと、今日の画面の「前回」が何か月も前のものになる。
 *
 * `enabled` は `useStats` と同じくトークンのため。無いまま叩くと 401 で
 * トークンが消える。履歴を開き直したら今月に戻すのにも使う。
 */
export function useMonthLogs(enabled: boolean, recent: readonly Day[]): MonthLogs {
  const [month, setMonth] = useState<Month>(() => monthOf(today()));
  const [fetched, setFetched] = useState<ReadonlyMap<Month, Day[]>>(new Map());
  const [failed, setFailed] = useState<Month | null>(null);

  useEffect(() => {
    if (enabled) setMonth(monthOf(today()));
  }, [enabled]);

  const load = useCallback(
    async (m: Month) => {
      const plan = planMonthLoad(m, today(), new Set(fetched.keys()));
      if (plan.kind !== 'fetch') return;
      setFailed(null);
      try {
        const res = await getJSON<HistoryResponse>(plan.path);
        setFetched((prev) => new Map(prev).set(m, res.days));
      } catch {
        setFailed(m);
      }
    },
    [fetched],
  );

  useEffect(() => {
    if (enabled) void load(month);
  }, [enabled, month, load]);

  const patch = useCallback((change: DayChange) => setFetched((f) => patchFetched(f, change)), []);

  const now = today();
  return {
    month,
    days: pickMonthDays(month, now, recent, fetched),
    error: failed === month ? 'この月の記録を読めませんでした' : '',
    canNext: canGoNext(month, now),
    prev: () => setMonth((m) => shiftMonth(m, -1)),
    next: () => setMonth((m) => (canGoNext(m, today()) ? shiftMonth(m, 1) : m)),
    reload: () => void load(month),
    patch,
  };
}
