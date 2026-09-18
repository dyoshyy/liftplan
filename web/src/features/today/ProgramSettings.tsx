import { useState } from 'react';
import { getJSON, send } from '../../api/client';
import type { Program } from '../../api/types';
import { focusBody, focusOptions, NO_FOCUS } from './focus';
import { lockedDeclared, toggleDeclared } from './declared';

type Props = {
  /** 種目IDを表示名にする。 */
  nameOf: (id: string) => string;
  /** 変更後にメニューを取り直す。設定はその日の献立を変える。 */
  onChanged: () => Promise<void>;
};

/**
 * 伸ばしたい種目と重点種目を変える。
 *
 * 畳んであるのは、ジムで開く画面の面積を増やさないため（D-120）。
 * プログラムを取りに行くのも開いたときだけで、毎回の読み込みには混ぜない。
 *
 * 2つを1つの部品にしているのは、どちらも同じプログラムを見ているため。
 * 別々に持つと、宣言を変えたあとに重点種目の選択肢が古いままになる。
 *
 * 待ち行列を通さずその場で送る。待ち行列は記録を守るための仕組みで、
 * 設定を混ぜると圏外で押した変更がジムを出たあとに流れ、その日の
 * メニューは変わらないまま「変えたつもり」になる。失敗しても記録は
 * 1件も失わないので、その場で成否を見せるほうが正直。
 */
export function ProgramSettings({ nameOf, onChanged }: Props) {
  const [open, setOpen] = useState(false);
  const [program, setProgram] = useState<Program | null>(null);
  const [note, setNote] = useState('');
  const [busy, setBusy] = useState(false);

  // 保存前のチェック状態。宣言はチェックを何個か動かしてから保存する。
  // 1つ動かすたびに送ると、そのたびにメニューが組み替わる。
  const [draft, setDraft] = useState<string[] | null>(null);

  const expand = async () => {
    setOpen(true);
    if (program) return;
    setNote('');
    try {
      const p = await getJSON<Program>('/api/program');
      setProgram(p);
      setDraft(p.declared_exercises);
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

  if (!open) {
    return (
      <button type="button" className="btn btn-quiet" onClick={() => void expand()}>
        メニューの設定を変える
      </button>
    );
  }

  const locked = program ? lockedDeclared(draft ?? [], program.focus_exercise) : new Map();
  const dirty =
    program && draft
      ? draft.join(',') !== [...program.declared_exercises].sort().join(',')
      : false;

  return (
    <div className="card">
      <p className="card-title">伸ばしたい種目</p>
      <p className="note mb-3">
        ここに入れた種目が、毎回1つずつ順に「軸」として出ます。最後にやったのが
        最も古いものが選ばれるので、数を増やすほど1種目あたりの頻度は下がります。
      </p>

      {program && draft && (
        <div className="grid gap-2">
          {program.selected_exercises.map((id) => {
            const on = draft.includes(id);
            const why = locked.get(id);
            return (
              <button
                key={id}
                type="button"
                className={`btn ${on ? '' : 'btn-quiet'}`}
                disabled={busy || (on && why !== undefined)}
                title={why}
                onClick={() => setDraft(toggleDeclared(draft, id))}
              >
                {on ? '✓ ' : ''}
                {nameOf(id)}
                {why && on ? ` — ${why}` : ''}
              </button>
            );
          })}
        </div>
      )}

      {dirty && (
        <button
          type="button"
          className="btn mt-3"
          disabled={busy}
          onClick={() => void saveDeclared()}
        >
          伸ばしたい種目を保存する
        </button>
      )}

      <p className="card-title mt-5">重点種目</p>
      <p className="note mb-3">
        選んだ種目の派生（ナローグリップ、テンポなど）が、軸とは別の枠で
        中1日以上あけて出ます。指定しなければバリエーションは出ません。
      </p>

      {program && (
        <div className="grid gap-2">
          {focusOptions(program.declared_exercises).map((id) => {
            const chosen = (program.focus_exercise ?? NO_FOCUS) === id;
            return (
              <button
                key={id || 'none'}
                type="button"
                className={`btn ${chosen ? '' : 'btn-quiet'}`}
                disabled={busy}
                onClick={() => void chooseFocus(id)}
              >
                {id === NO_FOCUS ? '指定しない' : nameOf(id)}
              </button>
            );
          })}
        </div>
      )}

      {note && <p className="mt-2.5 text-[13px] text-red">{note}</p>}
      <button type="button" className="btn btn-quiet mt-3" onClick={() => setOpen(false)}>
        閉じる
      </button>
    </div>
  );
}
