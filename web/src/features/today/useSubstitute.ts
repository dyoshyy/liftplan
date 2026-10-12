import { useCallback, useEffect, useRef, useState } from 'react';
import { getJSON } from '../../api/client';
import type { SubstituteResponse } from '../../api/types';
import { KEY_SUBSTITUTES, readJSON, writeJSON } from '../../storage/local';
import {
  decodeSwaps,
  encodeSwaps,
  rejectedOf,
  substituteErrorMessage,
  substituteQuery,
  swapsFor,
  withResult,
  type Swap,
  type SwapState,
} from './substitute';

export type Substitute = {
  /** swaps は表示している計画に対する差し替え。 */
  swaps: ReadonlyMap<string, Swap>;
  /** pending は取得中の補助（replace）。取得中はほかの押下を受けない。 */
  pending: string | null;
  /** errors は補助ごとの失敗の一言。 */
  errors: ReadonlyMap<string, string>;
  markBlocked: (replace: string) => Promise<void>;
};

/**
 * useSubstitute は「代替種目」を押したときの取得を持つ。
 *
 * 差し替えは localStorage に計画の日付つきで残す（設計書）。スマホは勝手に
 * 閉じるので、残さないと代わりの重量と目標セット数が開き直した時点で消える。
 * 別の日付のものは swapsFor が読まない。取得そのものは PWA のキャッシュにも
 * Outbox にも載せない。オフラインでは出さない（古い答えを見せない）。
 *
 * date は表示している計画の日付（Session.date）。端末の今日を使わない。
 */
export function useSubstitute(date: string | undefined): Substitute {
  const [state, setState] = useState<SwapState>(() => decodeSwaps(readJSON(KEY_SUBSTITUTES)));
  useEffect(() => writeJSON(KEY_SUBSTITUTES, encodeSwaps(state)), [state]);
  const [pending, setPending] = useState<string | null>(null);
  const [errors, setErrors] = useState<ReadonlyMap<string, string>>(new Map());
  // 連打の判定は ref で行う。state だと、押した直後の再描画より前の
  // 2回目の押下が古い pending（null）を読んで2回飛ぶ。
  const inflight = useRef(false);

  const swaps = date ? swapsFor(state, date) : new Map<string, Swap>();

  const markBlocked = useCallback(
    async (replace: string) => {
      if (!date || inflight.current) return;
      inflight.current = true;
      setPending(replace);
      setErrors((m) => {
        const next = new Map(m);
        next.delete(replace);
        return next;
      });
      try {
        const res = await getJSON<SubstituteResponse>(
          substituteQuery(date, replace, rejectedOf(swapsFor(state, date), replace)),
        );
        setState((s) => withResult(s, date, replace, res.substitute));
      } catch (e) {
        setErrors((m) => new Map(m).set(replace, substituteErrorMessage(e)));
      } finally {
        inflight.current = false;
        setPending(null);
      }
    },
    [date, state],
  );

  return { swaps, pending, errors, markBlocked };
}
