import { useCallback, useEffect, useState } from 'react';
import { send, Unauthorized } from '../../api/client';
import type { InvitePreview, JoinedGroup } from '../../api/types';
import { groupKeys, readJSON, writeJSON } from '../../storage/local';
import { describeFailure } from './groups';
import {
  keepsPending,
  planInviteIntake,
  viewOfPreview,
  type InviteView,
  type PreviewOutcome,
} from './invite';

export type PendingInvite = {
  /** 招待リンクを持っているか。ログインの前でも分かる。 */
  pending: boolean;
  view: InviteView;
  /** 参加を断られたときの一言。 */
  note: string;
  busy: boolean;
  join: (displayName: string) => Promise<void>;
  /** open は既に入っているグループを開く。 */
  open: () => void;
  dismiss: () => void;
};

const forget = () => {
  try {
    localStorage.removeItem(groupKeys.pendingInvite);
  } catch {
    // 消せなくても、次に開いたとき「参加しています」が出るだけ。
  }
};

/**
 * usePendingInvite は招待リンクを受け取り、ログインのあとで参加まで運ぶ。
 * 判断は invite.ts にある。
 *
 * 認証の戻り先は固定なので（open redirect の対策）、?invite= はログインの
 * 往復で落ちる。開いた時点で端末に覚え、ログインしてから中身を確かめる。
 */
export function usePendingInvite(hasToken: boolean, onEntered: (groupId: string) => void): PendingInvite {
  const [token, setToken] = useState<string | null>(() => readJSON<string>(groupKeys.pendingInvite));
  const [view, setView] = useState<InviteView>({ kind: 'none' });
  const [note, setNote] = useState('');
  const [busy, setBusy] = useState(false);

  // 開いたときの取り込み。何をどの順でやるかは planInviteIntake が決める。
  // history.state を渡し直すのは、useRoute が積んだ行き先を消さないため。
  useEffect(() => {
    for (const step of planInviteIntake(location.search)) {
      switch (step.kind) {
        case 'remember':
          writeJSON(groupKeys.pendingInvite, step.token);
          setToken(step.token);
          break;
        case 'clearQuery':
          history.replaceState(history.state, '', location.pathname + step.rest + location.hash);
          break;
      }
    }
  }, []);

  useEffect(() => {
    if (!hasToken || token === null) return;
    let stale = false;
    setView({ kind: 'loading' });
    preview(token).then((outcome) => {
      if (stale || outcome === null) return;
      const next = viewOfPreview(outcome);
      if (!keepsPending(next)) forget();
      setView(next);
    });
    return () => {
      stale = true;
    };
  }, [hasToken, token]);

  const close = useCallback(() => {
    forget();
    setToken(null);
    setView({ kind: 'none' });
    setNote('');
  }, []);

  const enter = (groupId: string) => {
    writeJSON(groupKeys.selected, groupId);
    close();
    onEntered(groupId);
  };

  const join = async (displayName: string) => {
    if (token === null) return;
    setBusy(true);
    setNote('');
    try {
      const res = await send({
        path: `/api/invites/${encodeURIComponent(token)}/join`,
        method: 'POST',
        body: { display_name: displayName },
      });
      if (res.status === 404) {
        forget();
        setView({ kind: 'invalid' });
        return;
      }
      if (!res.ok) {
        setNote(
          describeFailure(res.status, (await res.json().catch(() => null)) as { error?: string } | null),
        );
        return;
      }
      enter(((await res.json()) as JoinedGroup).group_id);
    } catch (e) {
      if (!(e instanceof Unauthorized)) setNote('つながらないので参加できません');
    } finally {
      setBusy(false);
    }
  };

  const open = () => {
    if (view.kind === 'already') enter(view.groupId);
  };

  return { pending: token !== null, view, note, busy, join, open, dismiss: close };
}

// preview は招待リンクの中身を取りに行く。401 は null（認証の側が扱う）。
async function preview(token: string): Promise<PreviewOutcome | null> {
  try {
    const res = await send({ path: `/api/invites/${encodeURIComponent(token)}` });
    if (!res.ok) return { kind: 'status', status: res.status };
    return { kind: 'ok', preview: (await res.json()) as InvitePreview };
  } catch (e) {
    return e instanceof Unauthorized ? null : { kind: 'network' };
  }
}
