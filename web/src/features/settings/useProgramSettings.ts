import { useEffect, useState } from 'react';
import { getJSON, send } from '../../api/client';
import type { Program, SplitPreset, SplitPresetsResponse } from '../../api/types';
import { focusBody, NO_FOCUS } from '../today/focus';
import { lockedDeclared } from '../today/declared';
import { matchingPresetKey, splitBody, splitUnselectableReason } from './split';

// 設定の判断。副作用は持たない。
//
// 送る前に決まることをここに集める。以前は ProgramSettings.tsx の中にあり、
// DOM を立てないと検査できなかった。

/** describePutFailure は PUT が失敗したときに画面へ出す1行を組む。
 *
 *  サーバーが本文に書いた理由をそのまま出す。5分割の頻度下限のように
 *  「なぜ成り立たないか」はサーバーしか知らない（どの分割にどの区分が
 *  あるか、頻度の下限）。状態コードだけでは本人が次に何をすればいいか
 *  分からない（web/src/dev/simulate.ts の describeFailure と同じ理由）。
 *  理由が無ければ状態コードだけを出す。 */
export const describePutFailure = (status: number, body: { error?: string } | null): string =>
  body?.error ? body.error : `変えられませんでした（${status}）`;

/** isDirty は種目の選択が変わったかを見る。
 *
 *  並び順の違いは無視する。並びだけで「変わった」にすると、押していないのに
 *  保存ボタンが出続ける。 */
export const isDirty = (draft: readonly string[], saved: readonly string[]): boolean =>
  [...draft].sort().join(',') !== [...saved].sort().join(',');

// 設定の手順を束ねる。
//
// **判断はこのファイル先頭と split.ts・../today/focus.ts・
// ../today/declared.ts に置き、ここは順に実行するだけ。**
//
// 待ち行列を通さずその場で送る。待ち行列は記録を守るための仕組みで、
// 設定を混ぜると圏外で押した変更がジムを出たあとに流れ、その日の
// メニューは変わらないまま「変えたつもり」になる。失敗しても記録は
// 1件も失わないので、その場で成否を見せるほうが正直。
export function useProgramSettings(onChanged: () => Promise<void>) {
  const [program, setProgram] = useState<Program | null>(null);
  const [note, setNote] = useState('');
  const [busy, setBusy] = useState(false);

  // 保存前のチェック状態。宣言はチェックを何個か動かしてから保存する。
  // 1つ動かすたびに送ると、そのたびにメニューが組み替わる。
  const [draft, setDraft] = useState<string[] | null>(null);
  const [pick, setPick] = useState<string[] | null>(null);
  const [presets, setPresets] = useState<SplitPreset[]>([]);

  // 画面を開いたら読む。以前は「開く」を押したときだけだったが、
  // 設定画面そのものが「開いた」の意味を持つようになった。
  useEffect(() => {
    void load();
    // load は描画ごとに作り直されるので、依存に入れると読むたびに読み直す。
    // load 自体は二度読みを見張らないので、初回だけに絞るのはこの [] の役目。
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  // 取りに行く。呼ぶのは画面を開いたときの1回だけ。
  //
  // 以前は保存のたびに手持ちを捨てて取り直していた。取り直すと、畳んだ節で
  // 動かしていた未保存のチェックまで保存済みの値で上書きされ、取り直すまで
  // 節の中身も空になる。保存した値は手元で書き換える（save* を参照）。
  const load = async () => {
    setNote('');
    try {
      const [p, sp] = await Promise.all([
        getJSON<Program>('/api/program'),
        getJSON<SplitPresetsResponse>('/api/split-presets'),
      ]);
      setProgram(p);
      setPresets(sp.presets);
      setDraft(p.declared_exercises);
      setPick(p.selected_exercises);
    } catch {
      setNote('設定を読めませんでした');
    }
  };

  // put は1フィールドだけの口へ送る。成否をそのまま返す。
  const put = async (path: string, body: unknown): Promise<boolean> => {
    if (!navigator.onLine) {
      setNote('つながらないので変えられません');
      return false;
    }
    setBusy(true);
    setNote('');
    try {
      const res = await send({ path, method: 'PUT', body });
      if (!res.ok) {
        const failure = (await res.json().catch(() => null)) as { error?: string } | null;
        setNote(describePutFailure(res.status, failure));
        return false;
      }
      return true;
    } catch {
      setNote('つながらないので変えられません');
      return false;
    } finally {
      setBusy(false);
    }
  };

  const chooseFocus = async (id: string) => {
    if (!program || busy) return;
    if (!(await put('/api/program/focus', focusBody(id)))) return;
    setProgram({ ...program, focus_exercise: id === NO_FOCUS ? null : id });
    await onChanged();
  };

  const saveDeclared = async () => {
    if (!program || !draft || busy) return;
    if (!(await put('/api/program/declared', { declared_exercises: draft }))) return;
    setProgram({ ...program, declared_exercises: draft });
    await onChanged();
  };

  const saveSelected = async () => {
    if (!program || !pick || busy) return;
    if (!(await put('/api/program/selected', { selected_exercises: pick }))) return;
    setProgram({ ...program, selected_exercises: pick });
    await onChanged();
  };

  const chooseSplit = async (preset: SplitPreset | null) => {
    if (!program || busy) return;
    if (!(await put('/api/program/split', splitBody(preset)))) return;
    setProgram({ ...program, splits: preset ? preset.splits : [] });
    await onChanged();
  };

  // 週目標を読んでいたころは、回数と量を変えると週目標が置き直るので
  // 取り直していた。いまは画面が週目標を読まない（#176）ので、ほかの
  // save* と同じく手元を書き換えるだけでよい。
  const saveFrequency = async (n: number) => {
    if (!program || busy) return;
    if (!(await put('/api/program/frequency', { per_week: n }))) return;
    setProgram({ ...program, per_week: n });
    await onChanged();
  };

  const saveVolume = async (exercises: number, sets: number) => {
    if (!program || busy) return;
    const body = { exercises_per_session: exercises, sets_per_exercise: sets };
    if (!(await put('/api/program/volume', body))) return;
    setProgram({ ...program, ...body });
    await onChanged();
  };

  const locked = program ? lockedDeclared(draft ?? [], program.focus_exercise) : new Map();
  const dirty = program && draft ? isDirty(draft, program.declared_exercises) : false;
  const pickDirty = program && pick ? isDirty(pick, program.selected_exercises) : false;
  const splitKey = program ? matchingPresetKey(program, presets) : null;
  // プリセットごとに「いまの頻度で選べるか」を添える。5分割のように
  // 下限を持つプリセットは、頻度が足りない間ボタンを押させない
  // （押しても保存の口が同じ理由で拒否するので、実害は無いが、
  // 押す前に理由が読めたほうが本人が次に何をすればいいか分かる）。
  const splitOptions = presets.map((p) => ({
    preset: p,
    reason: program ? splitUnselectableReason(p, program.per_week) : null,
  }));

  return {
    program,
    draft,
    setDraft,
    pick,
    setPick,
    note,
    busy,
    locked,
    dirty,
    pickDirty,
    chooseFocus,
    saveDeclared,
    saveSelected,
    saveFrequency,
    saveVolume,
    splitOptions,
    splitKey,
    chooseSplit,
  };
}
