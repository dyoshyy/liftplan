import { useState } from 'react';
import { getJSON, send } from '../../api/client';
import type { Program } from '../../api/types';
import { focusBody, focusOptions, NO_FOCUS } from './focus';

type Props = {
  /** 種目IDを表示名にする。 */
  nameOf: (id: string) => string;
  /** 変更後にメニューを取り直す。重点種目はその日の献立を変える。 */
  onChanged: () => Promise<void>;
};

/**
 * 重点種目を選ぶ。宣言種目のうち1つ、または指定なし。
 *
 * 畳んであるのは、ジムで開く画面の面積を増やさないため（D-120）。
 * プログラムを取りに行くのも開いたときだけで、毎回の読み込みには混ぜない。
 *
 * 待ち行列を通さずその場で送る。待ち行列は記録を守るための仕組みで、
 * 設定を混ぜると圏外で押した変更がジムを出たあとに流れ、その日の
 * メニューは変わらないまま「変えたつもり」になる。失敗しても記録は
 * 1件も失わないので、その場で成否を見せて選択を戻すほうが正直（D-127）。
 */
export function FocusPicker({ nameOf, onChanged }: Props) {
  const [open, setOpen] = useState(false);
  const [program, setProgram] = useState<Program | null>(null);
  const [note, setNote] = useState('');
  const [busy, setBusy] = useState(false);

  const expand = async () => {
    setOpen(true);
    if (program) return;
    setNote('');
    try {
      setProgram(await getJSON<Program>('/api/program'));
    } catch {
      setNote('設定を読めませんでした');
    }
  };

  const choose = async (id: string) => {
    if (!program || busy) return;
    if (!navigator.onLine) {
      setNote('つながらないので変えられません');
      return;
    }

    setBusy(true);
    setNote('');
    try {
      const res = await send({
        path: '/api/program/focus',
        method: 'PUT',
        body: focusBody(id),
      });
      if (!res.ok) {
        setNote(`変えられませんでした（${res.status}）`);
        return;
      }
      setProgram({ ...program, focus_exercise: id === NO_FOCUS ? null : id });
      await onChanged();
    } catch {
      setNote('つながらないので変えられません');
    } finally {
      setBusy(false);
    }
  };

  if (!open) {
    return (
      <button type="button" className="btn btn-quiet" onClick={() => void expand()}>
        重点種目を変える
      </button>
    );
  }

  return (
    <div className="card">
      <p className="card-title">重点種目</p>
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
                onClick={() => void choose(id)}
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
