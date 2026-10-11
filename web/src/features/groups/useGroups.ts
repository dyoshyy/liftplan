import { useCallback, useEffect, useState } from 'react';
import { getJSON, send } from '../../api/client';
import type {
  CreatedGroup,
  GroupSummary,
  GroupsResponse,
  GroupWeek,
  InviteTokenResponse,
} from '../../api/types';
import { today } from '../../domain/date';
import { groupKeys, readJSON, writeJSON } from '../../storage/local';
import { chooseGroup, describeFailure, groupErrorMessage } from './groups';

export type Groups = {
  /** 自分のグループ。読めていなければ null。 */
  groups: GroupSummary[] | null;
  /** 開いているグループの今週。 */
  week: GroupWeek | null;
  selectedId: string | null;
  /** 読めなかったときの一言。空なら出さない。 */
  error: string;
  /** 送って断られたときの一言。 */
  note: string;
  busy: boolean;
  select: (id: string) => void;
  create: (name: string, displayName: string) => Promise<boolean>;
  rotate: () => Promise<void>;
  leave: () => Promise<void>;
};

/**
 * useGroups はグループの画面の手順を束ねる。判断は groups.ts にある。
 *
 * 開いたときに読む（useForecast と同じ形）。送信待ちの列には乗せない。
 * 読むだけの画面で、つながっていなければ見せないほうが正直
 * （設計書「招待リンクの流れ」の最後）。
 */
export function useGroups(): Groups {
  const [groups, setGroups] = useState<GroupSummary[] | null>(null);
  const [selectedId, setSelectedId] = useState<string | null>(null);
  const [week, setWeek] = useState<GroupWeek | null>(null);
  const [error, setError] = useState('');
  const [note, setNote] = useState('');
  const [busy, setBusy] = useState(false);

  const loadGroups = useCallback(async (prefer: string | null) => {
    setError('');
    try {
      const res = await getJSON<GroupsResponse>('/api/groups');
      setGroups(res.groups);
      setSelectedId(chooseGroup(res.groups, prefer ?? readJSON<string>(groupKeys.selected)));
    } catch (e) {
      setError(groupErrorMessage(e));
    }
  }, []);

  useEffect(() => {
    void loadGroups(null);
  }, [loadGroups]);

  useEffect(() => {
    setWeek(null);
    if (selectedId === null) return;
    writeJSON(groupKeys.selected, selectedId);
    let stale = false;
    getJSON<GroupWeek>(`/api/groups/${encodeURIComponent(selectedId)}/week?date=${today()}`)
      .then((w) => {
        if (!stale) setWeek(w);
      })
      .catch((e: unknown) => {
        if (!stale) setError(groupErrorMessage(e));
      });
    // 切り替えた直後に前のグループの応答が着いても、上書きさせない。
    return () => {
      stale = true;
    };
  }, [selectedId]);

  // request はその場で送る。断られたら理由を note に置いて null を返す。
  const request = async <T>(path: string, method: string, body?: unknown): Promise<T | null> => {
    setBusy(true);
    setNote('');
    try {
      const res = await send({ path, method, body });
      if (!res.ok) {
        setNote(
          describeFailure(res.status, (await res.json().catch(() => null)) as { error?: string } | null),
        );
        return null;
      }
      return res.status === 204 ? ({} as T) : ((await res.json()) as T);
    } catch {
      setNote('つながらないのでできません');
      return null;
    } finally {
      setBusy(false);
    }
  };

  const create = async (name: string, displayName: string): Promise<boolean> => {
    const created = await request<CreatedGroup>('/api/groups', 'POST', { name, display_name: displayName });
    if (!created) return false;
    await loadGroups(created.id);
    return true;
  };

  const rotate = async (): Promise<void> => {
    if (selectedId === null) return;
    const res = await request<InviteTokenResponse>(
      `/api/groups/${encodeURIComponent(selectedId)}/invite`,
      'POST',
    );
    if (res) setWeek((w) => (w ? { ...w, invite_token: res.invite_token } : w));
  };

  const leave = async (): Promise<void> => {
    if (selectedId === null) return;
    const res = await request(`/api/groups/${encodeURIComponent(selectedId)}/members/me`, 'DELETE');
    if (res) await loadGroups(null);
  };

  return { groups, week, selectedId, error, note, busy, select: setSelectedId, create, rotate, leave };
}
