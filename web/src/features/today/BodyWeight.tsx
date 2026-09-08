import { useState } from 'react';
import { today } from '../../domain/date';
import type { ConditionInput } from '../../api/types';
import type { QueueItem } from '../../outbox/db';

// 体重だけを記録する。睡眠は落とした。
//
// 睡眠はサーバーのどの判断にも使われていない（deload_policy も
// session_planner も参照していない）。
//
// **体重は落とせない。**デロードの判定だけでなく、自重種目
// （ディップス・懸垂・バックエクステンション）の処方と推定に使われる。
// しかも体重が一件も無い間は既定 70kg で動くので、数週間ぶん記録した
// あとに体重を入れ始めると、それ以前の自重種目が推定から除外される。
// 最初から入れておかないと、あとで推移が失われる。
export function BodyWeight({ enqueue }: { enqueue: (item: QueueItem) => Promise<void> }) {
  const [value, setValue] = useState('');
  const [note, setNote] = useState('');

  const save = async () => {
    const bw = Number.parseFloat(value);
    if (!Number.isFinite(bw) || bw <= 0) {
      setNote('体重を入れてください');
      return;
    }
    setNote('');
    const item: ConditionInput = { date: today(), body_weight_kg: bw };
    await enqueue({ path: '/api/conditions', body: { conditions: [item] } });
    setValue('');
  };

  return (
    <div className="card">
      <p className="card-title">今日の体重</p>
      <div className="field">
        <label htmlFor="bw">体重 (kg)</label>
        <input
          id="bw"
          type="number"
          inputMode="decimal"
          step="0.1"
          placeholder="75.0"
          value={value}
          onChange={(e) => setValue(e.target.value)}
        />
      </div>
      <button type="button" className="btn btn-quiet mt-3" onClick={() => void save()}>
        記録する
      </button>
      {note && <p className="mt-2.5 text-[13px] text-red">{note}</p>}
      <p className="note mt-2.5">
        懸垂やディップスの重量は体重から計算されます。最初から入れておかないと、
        あとで入れたときに、それまでの推移が計算から外れます。
      </p>
    </div>
  );
}
