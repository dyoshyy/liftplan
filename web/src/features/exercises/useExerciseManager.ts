import { useEffect, useState } from 'react';
import { getJSON, send } from '../../api/client';
import type { Exercise, Program } from '../../api/types';
import { describePutFailure } from '../settings/useProgramSettings';
import { draftBody, draftProblem, nextDeleteStep, type ExerciseDraft } from './exerciseDraft';

// 種目を管理する画面の手順を束ねる。
//
// 判断（送れるか・本文・消せるか・要約）は exerciseDraft.ts に置き、ここは
// 順に実行するだけ。種目マスタそのもの（一覧）はこの画面の外（useLiftplan）
// が持っていて、送った後は onChanged で取り直してもらう。
//
// 伸ばしたい種目（消せない理由の判定に要る）は、設定の状態を受け取らず
// ここで自分で読む。設定を経由せずに開くこともある別ページなので、
// 設定の state を貫通させるより、必要なものをここで取りに行くほうが
// 依存が浅い。
export function useExerciseManager(onChanged: () => Promise<void>) {
  const [declared, setDeclared] = useState<string[]>([]);
  const [note, setNote] = useState('');
  const [busy, setBusy] = useState(false);
  // pendingDeleteId は「消す」の確認待ちの種目。消した種目は直せない・
  // 戻せない（undo が無い）ので、直すの隣の1タップで即消えないよう、同じ
  // 種目をもう一度押すまでは実際には消さない（nextDeleteStep）。
  const [pendingDeleteId, setPendingDeleteId] = useState<string | null>(null);

  useEffect(() => {
    void getJSON<Program>('/api/program')
      .then((p) => setDeclared(p.declared_exercises))
      .catch(() => setNote('伸ばしたい種目を読めませんでした'));
    // 開いたときの1回だけ読む。以後は declared が変わっても（設定側で
    // 触られても）この画面を開き直すまでは古いままでよい。押す前に
    // 読める案内でしかなく、最後の砦はサーバーの409。
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  // request はその場で送る。成否は describePutFailure が組む1行を note に置く。
  // useProgramSettings の request と同じ形（サーバーの理由をそのまま出す）。
  const request = async (path: string, method: string, body?: unknown): Promise<Response | null> => {
    if (!navigator.onLine) {
      setNote('つながらないので変えられません');
      return null;
    }
    setBusy(true);
    setNote('');
    try {
      const res = await send({ path, method, body });
      if (!res.ok) {
        const failure = (await res.json().catch(() => null)) as { error?: string } | null;
        setNote(describePutFailure(res.status, failure));
        return null;
      }
      return res;
    } catch {
      setNote('つながらないので変えられません');
      return null;
    } finally {
      setBusy(false);
    }
  };

  // add は POST /api/exercises。足せたかを返す。書きかけを消すかは呼び手が決める。
  const add = async (draft: ExerciseDraft): Promise<boolean> => {
    if (busy) return false;
    const problem = draftProblem(draft);
    if (problem) {
      setNote(problem);
      return false;
    }
    const res = await request('/api/exercises', 'POST', draftBody(draft));
    if (!res) return false;
    await onChanged();
    return true;
  };

  // edit は PUT /api/exercises/{id}。
  const edit = async (id: string, draft: ExerciseDraft): Promise<boolean> => {
    if (busy) return false;
    const problem = draftProblem(draft);
    if (problem) {
      setNote(problem);
      return false;
    }
    const res = await request(`/api/exercises/${encodeURIComponent(id)}`, 'PUT', draftBody(draft));
    if (!res) return false;
    await onChanged();
    return true;
  };

  // remove は DELETE /api/exercises/{id}。伸ばしたい種目なら409（サーバーが断る）。
  const remove = async (id: string): Promise<boolean> => {
    if (busy) return false;
    const res = await request(`/api/exercises/${encodeURIComponent(id)}`, 'DELETE');
    if (!res) return false;
    await onChanged();
    return true;
  };

  // requestDelete は「消す」ボタンの送信口。決めるのは nextDeleteStep（判断）
  // だけで、ここは決まった一手を実行するだけ（3層：判断は exerciseDraft.ts、
  // 手順はここ、描画は ExerciseManager.tsx）。1タップ目は確認待ちに入るだけで
  // 何も送らない。確認待ちの種目をもう一度押したときだけ実際に消す。
  const requestDelete = async (id: string): Promise<boolean> => {
    const step = nextDeleteStep(pendingDeleteId, id);
    setPendingDeleteId(step.pendingId);
    if (step.act !== 'confirm') return false;
    return remove(id);
  };

  // cancelDelete は確認待ちを解く（「やめる」、もしくは足す・直すを開いたとき）。
  const cancelDelete = () => setPendingDeleteId(null);

  // save は編集フォームの送信口。'new' か直す対象の種目かで add/edit に振り分ける。
  //
  // 描画（ExerciseManager.tsx）に分岐と await を持たせないための置き場所
  // （3層：判断は exerciseDraft.ts、手順はここ、描画は ExerciseManager.tsx）。
  const save = (target: 'new' | Exercise, draft: ExerciseDraft): Promise<boolean> =>
    target === 'new' ? add(draft) : edit(target.id, draft);

  return { declared, note, busy, save, pendingDeleteId, requestDelete, cancelDelete };
}
