import { useEffect, useState } from 'react';
import { getJSON, send } from '../../api/client';
import type { Program, SplitPreset, SplitPresetsResponse } from '../../api/types';
import { focusBody, NO_FOCUS } from '../today/focus';
import { lockedDeclared } from '../today/declared';
import { matchingPresetKey, splitBody } from './split';

// 設定の判断。副作用は持たない。
//
// 送る前に決まることをここに集める。以前は ProgramSettings.tsx の中にあり、
// DOM を立てないと検査できなかった。

/** isDirty は種目の選択が変わったかを見る。
 *
 *  並び順の違いは無視する。並びだけで「変わった」にすると、押していないのに
 *  保存ボタンが出続ける。 */
export const isDirty = (draft: readonly string[], saved: readonly string[]): boolean =>
  [...draft].sort().join(',') !== [...saved].sort().join(',');

// 設定の手順を束ねる。
//
// **判断は program.ts に置き、ここは順に実行するだけ。**
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
    // load は program を見て二度読みを避けるだけなので、初回に1回でよい。
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  // 取りに行く。呼ばれたら必ず取る。
  //
  // 以前は `if (program) return;` で二度読みを避けていたが、この関数は
  // 描画時の program を掴んでいるので、setProgram(null) の直後に呼んでも
  // 古い値を見て即座に抜けていた。**週に通う回数を変えると設定画面が
  // 空白になり、開き直すまで戻らなかった。**
  //
  // 初回の1回だけ読めばよいのは useEffect の依存配列が保証するので、
  // ここで重ねて見張る必要がない。
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
        setNote(`変えられませんでした（${res.status}）`);
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

  const saveFrequency = async (n: number) => {
    if (!program || busy) return;
    if (!(await put('/api/program/frequency', { per_week: n }))) return;
    // 画面の手持ちは捨てて取り直す。
    setProgram(null);
    setDraft(null);
    setPick(null);
    await onChanged();
    await load();
  };

  const locked = program ? lockedDeclared(draft ?? [], program.focus_exercise) : new Map();
  const dirty = program && draft ? isDirty(draft, program.declared_exercises) : false;
  const pickDirty = program && pick ? isDirty(pick, program.selected_exercises) : false;
  const splitKey = program ? matchingPresetKey(program, presets) : null;


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
    presets,
    splitKey,
    chooseSplit,
  };
}
