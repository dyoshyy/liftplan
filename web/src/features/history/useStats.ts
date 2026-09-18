import { useCallback, useEffect, useState } from 'react';
import { getJSON } from '../../api/client';
import type { StatsResponse } from '../../api/types';
import { addDays, today } from '../../domain/date';

/** WINDOW_DAYS は遡る日数。サーバーの defaultHistoryDays と同じ8週間。 */
const WINDOW_DAYS = 56;

export type Stats = {
  stats: StatsResponse | null;
  /** 読めなかったときの一言。空なら成功。 */
  error: string;
  reload: () => Promise<void>;
};

/**
 * useStats は週の充足と推定1RM の推移を取りに行く。
 *
 * **画面の部品から fetch を外に出してある。**`History` は props だけで
 * 描けるので、呼び出し側が「いつ取るか」を決められる。履歴を常に読むか、
 * 開いたときだけ読むかは、置き場所が決まってからでよい。
 *
 * `enabled` が真になった時点で1回読む。読み直しは `reload` で明示的に。
 * 自動で取り直すと、見ている最中に数字が入れ替わる。
 *
 * **`enabled` を持たせてあるのはトークンのため。**トークンが無いまま
 * 叩くと 401 が返り、`api/client` はそれを「トークンが通らなかった」と
 * みなして端末から消す。まだ入れていないだけなのに、入れた直後の
 * 再読み込みまで巻き込まれる。実機で踏んで気づいた（ログイン前に
 * 描画すると「履歴を読めませんでした」のまま戻らない）。
 */
export function useStats(enabled = true): Stats {
  const [stats, setStats] = useState<StatsResponse | null>(null);
  const [error, setError] = useState('');

  const reload = useCallback(async () => {
    if (!enabled) return;
    setError('');
    const to = today();
    try {
      setStats(await getJSON<StatsResponse>(`/api/stats?from=${addDays(to, -WINDOW_DAYS)}&to=${to}`));
    } catch {
      setError('履歴を読めませんでした');
    }
  }, [enabled]);

  useEffect(() => {
    void reload();
  }, [reload]);

  return { stats, error, reload };
}
