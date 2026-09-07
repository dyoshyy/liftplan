import { useState } from 'react';
import { today } from '../../domain/date';
import type { ConditionInput } from '../../api/types';
import type { QueueItem } from '../../outbox/db';

export function Condition({ enqueue }: { enqueue: (item: QueueItem) => Promise<void> }) {
  const [bodyWeight, setBodyWeight] = useState('');
  const [sleep, setSleep] = useState('');
  const [note, setNote] = useState('');

  const save = async () => {
    const bw = Number.parseFloat(bodyWeight);
    const sl = Number.parseFloat(sleep);
    const item: ConditionInput = { date: today() };
    if (Number.isFinite(bw)) item.body_weight_kg = bw;
    if (Number.isFinite(sl)) item.sleep_hours = sl;

    if (item.body_weight_kg === undefined && item.sleep_hours === undefined) {
      setNote('体重か睡眠のどちらかを入れてください');
      return;
    }
    setNote('');
    await enqueue({ path: '/api/conditions', body: { conditions: [item] } });
    setBodyWeight('');
    setSleep('');
  };

  return (
    <div className="card">
      <p className="card-title">今日のコンディション</p>
      <div className="grid grid-cols-2 gap-3">
        <div className="field">
          <label htmlFor="bw">体重 (kg)</label>
          <input
            id="bw"
            type="number"
            inputMode="decimal"
            step="0.1"
            placeholder="75.0"
            value={bodyWeight}
            onChange={(e) => setBodyWeight(e.target.value)}
          />
        </div>
        <div className="field">
          <label htmlFor="sl">睡眠 (時間)</label>
          <input
            id="sl"
            type="number"
            inputMode="decimal"
            step="0.25"
            placeholder="7.5"
            value={sleep}
            onChange={(e) => setSleep(e.target.value)}
          />
        </div>
      </div>
      <button type="button" className="btn btn-quiet mt-3" onClick={() => void save()}>
        記録する
      </button>
      {note && <p className="mt-2.5 text-[13px] text-red">{note}</p>}
      <p className="note mt-2.5">
        体重が無いとデロードの判定ができません。減量による停滞と、追い込みすぎによる停滞が
        同じ形をしているためです。
      </p>
    </div>
  );
}
