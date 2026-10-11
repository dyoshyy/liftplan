import { useEffect, useState } from 'react';
import { getJSON } from '../../api/client';
import type { GroupWeek, InviteTokenResponse } from '../../api/types';
import { today } from '../../domain/date';
import { groupErrorMessage } from './groups';
import { sendGroupRequest } from './request';

export type GroupWeekState = {
  week: GroupWeek | null;
  /** 読めなかったときの一言。空なら出さない。 */
  error: string;
  /** 送って断られたときの一言。 */
  note: string;
  busy: boolean;
  rotate: () => Promise<void>;
  /** leave は抜けられたかを返す。 */
  leave: () => Promise<boolean>;
};

/** useGroupWeek は1つのグループの今週と、そのグループへの操作を束ねる。 */
export function useGroupWeek(id: string): GroupWeekState {
  const [week, setWeek] = useState<GroupWeek | null>(null);
  const [error, setError] = useState('');
  const [note, setNote] = useState('');
  const [busy, setBusy] = useState(false);
  const base = `/api/groups/${encodeURIComponent(id)}`;

  useEffect(() => {
    let stale = false;
    setWeek(null);
    setError('');
    getJSON<GroupWeek>(`${base}/week?date=${today()}`)
      .then((w) => {
        if (!stale) setWeek(w);
      })
      .catch((e: unknown) => {
        if (!stale) setError(groupErrorMessage(e));
      });
    // 別のグループへ移った直後に前の応答が着いても、上書きさせない。
    return () => {
      stale = true;
    };
  }, [base]);

  const run = async <T>(path: string, method: string): Promise<T | null> => {
    setBusy(true);
    setNote('');
    const sent = await sendGroupRequest<T>(path, method);
    setBusy(false);
    if (!sent.ok) {
      setNote(sent.note);
      return null;
    }
    return sent.value;
  };

  const rotate = async () => {
    const res = await run<InviteTokenResponse>(`${base}/invite`, 'POST');
    if (res) setWeek((w) => (w ? { ...w, invite_token: res.invite_token } : w));
  };

  const leave = async () => (await run(`${base}/members/me`, 'DELETE')) !== null;

  return { week, error, note, busy, rotate, leave };
}
