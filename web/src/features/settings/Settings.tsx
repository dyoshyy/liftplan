import { useState } from 'react';
import type { Exercise, Program } from '../../api/types';
import { send } from '../../api/client';
import { clearToken } from '../../storage/local';

type Props = {
  program: Program | null;
  exercises: Exercise[];
  onSaved: () => void;
  onForget: () => void;
};

export function Settings({ program, exercises, onSaved, onForget }: Props) {
  const [perWeek, setPerWeek] = useState(String(program?.per_week ?? 3));
  const [selected, setSelected] = useState(new Set(program?.selected_exercises ?? []));
  const [message, setMessage] = useState('ここで選んだ種目だけが提案されます。');
  const [confirming, setConfirming] = useState(false);

  const toggle = (id: string) => {
    const next = new Set(selected);
    if (next.has(id)) next.delete(id);
    else next.add(id);
    setSelected(next);
  };

  const save = async () => {
    if (!program) return;
    const res = await send({
      path: '/api/program',
      method: 'PUT',
      body: {
        per_week: Number.parseInt(perWeek, 10),
        weekly_target: program.weekly_target,
        selected_exercises: [...selected],
      },
    });
    if (res.ok) {
      setMessage('保存しました');
      onSaved();
      return;
    }
    const err = (await res.json().catch(() => ({}))) as { error?: string };
    setMessage(err.error ?? `保存に失敗しました (${res.status})`);
  };

  return (
    <div className="grid gap-3.5">
      <div className="card">
        <p className="card-title">プログラム</p>
        <div className="field mb-3">
          <label htmlFor="per-week">週の頻度</label>
          <select id="per-week" value={perWeek} onChange={(e) => setPerWeek(e.target.value)}>
            {[1, 2, 3, 4].map((n) => (
              <option key={n} value={n}>
                週{n}回
              </option>
            ))}
          </select>
        </div>

        <p className="card-title mt-4">使う種目</p>
        <div className="flex flex-wrap gap-[7px]">
          {exercises.map((e) => (
            <button
              key={e.id}
              type="button"
              className="chip"
              aria-pressed={selected.has(e.id)}
              onClick={() => toggle(e.id)}
            >
              {e.name}
            </button>
          ))}
        </div>

        <button type="button" className="btn mt-4" onClick={() => void save()}>
          保存する
        </button>
        <p className="note mt-2.5">{message}</p>
      </div>

      <div className="card">
        <p className="card-title">この端末</p>
        {/* confirm() はページ全体を止めるので使わない。
            取り消しの効かない操作は、その場で二段階にする。 */}
        {confirming ? (
          <>
            <p className="note mb-3">
              この端末からトークンを消します。未送信の記録は消えませんが、送れなくなります。
            </p>
            <button
              type="button"
              className="btn btn-danger"
              onClick={() => {
                clearToken();
                onForget();
              }}
            >
              消す
            </button>
            <button type="button" className="btn btn-quiet mt-2" onClick={() => setConfirming(false)}>
              やめる
            </button>
          </>
        ) : (
          <button type="button" className="btn btn-danger" onClick={() => setConfirming(true)}>
            トークンを消す
          </button>
        )}
      </div>
    </div>
  );
}
