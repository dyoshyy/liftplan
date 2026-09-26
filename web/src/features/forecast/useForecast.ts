import { useCallback, useEffect, useState } from 'react';
import { getJSON } from '../../api/client';
import type { ForecastResponse, ForecastSession } from '../../api/types';
import { today } from '../../domain/date';

export type Forecast = {
  sessions: ForecastSession[] | null;
  /** 読めなかったときの一言。空なら成功。 */
  error: string;
  reload: () => Promise<void>;
};

/**
 * useForecast はこの先の予定を取りに行く。
 *
 * 毎回見るものではないので、今日のメニュー（useLiftplan）の読み込みには
 * 混ぜない。ページが開かれたとき（マウント時）に1回取りに行く
 * （useStats と同じ形）。PWA のキャッシュにも Outbox にも載せない
 * （設計書の決定）。古い見込みを見せるより、オフラインでは出さない
 * ほうが正直。
 */
export function useForecast(): Forecast {
  const [sessions, setSessions] = useState<ForecastSession[] | null>(null);
  const [error, setError] = useState('');

  const reload = useCallback(async () => {
    setError('');
    try {
      const res = await getJSON<ForecastResponse>(`/api/sessions/forecast?date=${today()}`);
      setSessions(res.sessions);
    } catch {
      setSessions(null);
      setError('オフラインでは見られません');
    }
  }, []);

  useEffect(() => {
    void reload();
  }, [reload]);

  return { sessions, error, reload };
}
