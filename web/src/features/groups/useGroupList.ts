import { useCallback, useEffect, useState } from 'react';
import { getJSON } from '../../api/client';
import type { CreatedGroup, GroupSummary, GroupsResponse } from '../../api/types';
import { groupErrorMessage } from './groups';
import { sendGroupRequest } from './request';

export type GroupList = {
  /** 自分のグループ。読めていなければ null。 */
  groups: GroupSummary[] | null;
  /** 読めなかったときの一言。空なら出さない。 */
  error: string;
  /** 送って断られたときの一言。 */
  note: string;
  busy: boolean;
  /** create は作ったグループの ID を返す。作れなければ null。 */
  create: (name: string, displayName: string) => Promise<string | null>;
};

/**
 * useGroupList はグループの一覧の手順を束ねる。判断は groups.ts にある。
 *
 * 開いたときに読む（useForecast と同じ形）。送信待ちの列には乗せない。
 */
export function useGroupList(): GroupList {
  const [groups, setGroups] = useState<GroupSummary[] | null>(null);
  const [error, setError] = useState('');
  const [note, setNote] = useState('');
  const [busy, setBusy] = useState(false);

  const load = useCallback(async () => {
    setError('');
    try {
      setGroups((await getJSON<GroupsResponse>('/api/groups')).groups);
    } catch (e) {
      setError(groupErrorMessage(e));
    }
  }, []);

  useEffect(() => {
    void load();
  }, [load]);

  const create = async (name: string, displayName: string): Promise<string | null> => {
    setBusy(true);
    setNote('');
    const sent = await sendGroupRequest<CreatedGroup>('/api/groups', 'POST', {
      name,
      display_name: displayName,
    });
    setBusy(false);
    if (!sent.ok) {
      setNote(sent.note);
      return null;
    }
    return sent.value.id;
  };

  return { groups, error, note, busy, create };
}
