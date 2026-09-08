import { useEffect, useRef, useState } from 'react';
import type { RecordedSet } from '../../api/types';
import type { LastPerformance } from '../../domain/sets';
import type { CardPlan } from './ExerciseCard';

export type SheetTarget = {
  plan: CardPlan;
  index: number;
  recorded: RecordedSet | undefined;
};

type Props = {
  target: SheetTarget;
  name: string;
  last: LastPerformance | undefined;
  onRecord: (values: { weight: number; reps: number; rir: number }) => void;
  onUndo: () => void;
  onClose: () => void;
};

export function RecordSheet({ target, name, last, onRecord, onUndo, onClose }: Props) {
  const { plan, index, recorded } = target;
  const ref = useRef<HTMLDialogElement>(null);

  const [weight, setWeight] = useState(
    String(recorded?.weight_kg ?? plan.weight_kg ?? last?.weight_kg ?? ''),
  );
  const [reps, setReps] = useState(String(recorded?.reps ?? last?.reps[index] ?? 8));
  const [rir, setRir] = useState(String(recorded?.rir ?? plan.target_rir));
  const [warning, setWarning] = useState('');

  useEffect(() => {
    ref.current?.showModal();
  }, []);

  const submit = () => {
    const w = Number.parseFloat(weight);
    const r = Number.parseInt(reps, 10);
    const i = Number.parseInt(rir, 10);
    if (!Number.isFinite(w) || !Number.isInteger(r) || !Number.isInteger(i)) {
      setWarning('重量・レップ・RIR を入れてください');
      return;
    }
    if (w <= 0 || r <= 0 || i < 0) {
      setWarning('0 より大きい重量とレップを入れてください');
      return;
    }
    onRecord({ weight: w, reps: r, rir: i });
  };

  return (
    <dialog ref={ref} onClose={onClose} onCancel={onClose}>
      <div
        className="grid gap-3.5 px-4 pt-[18px]"
        style={{ paddingBottom: 'calc(20px + env(safe-area-inset-bottom))' }}
      >
        <div className="flex items-center gap-2.5">
          <span className="font-bold">
            {name} {index + 1}セット目
          </span>
          {last && (
            <span className="ml-auto text-xs text-faint">
              前回 {last.weights?.[index] ?? last.weight_kg}kg × {last.reps[index] ?? '–'}
            </span>
          )}
        </div>

        <Stepper label="重量 (kg)" value={weight} onChange={setWeight} step={2.5} decimal />
        <Stepper label="レップ（実際にできた回数）" value={reps} onChange={setReps} step={1} />
        <Stepper label="RIR（あと何回できたか）" value={rir} onChange={setRir} step={1} />

        {/* alert() はページ全体を止めてしまうので、注意はその場に出す。
            直したいのはシートの中の値なので、シートを開いたまま伝える。 */}
        {warning && <p className="m-0 text-[13px] text-red">{warning}</p>}

        <button type="button" className="btn" onClick={submit}>
          記録する
        </button>
        {recorded && (
          <button type="button" className="btn btn-danger" onClick={onUndo}>
            この記録を取り消す
          </button>
        )}
        <button type="button" className="btn btn-quiet" onClick={() => ref.current?.close()}>
          閉じる
        </button>
      </div>
    </dialog>
  );
}

// Stepper は数値入力にボタンを添える。
//
// ブラウザ既定のスピナーは指で押せる大きさにならない。汗ばんだ手でも
// 押せる大きさが要る。
function Stepper({
  label,
  value,
  onChange,
  step,
  decimal,
}: {
  label: string;
  value: string;
  onChange: (v: string) => void;
  step: number;
  decimal?: boolean;
}) {
  const bump = (by: number) => {
    const now = Number.parseFloat(value);
    const next = (Number.isFinite(now) ? now : 0) + by;
    onChange(String(Math.max(0, Math.round(next * 100) / 100)));
  };

  return (
    <div className="field">
      <label>{label}</label>
      <div className="stepper">
        <button type="button" onClick={() => bump(-step)}>
          −
        </button>
        <input
          type="number"
          inputMode={decimal ? 'decimal' : 'numeric'}
          step={decimal ? 0.5 : 1}
          value={value}
          onChange={(e) => onChange(e.target.value)}
        />
        <button type="button" onClick={() => bump(step)}>
          ＋
        </button>
      </div>
    </div>
  );
}
