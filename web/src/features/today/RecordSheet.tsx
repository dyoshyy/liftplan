import { useEffect, useRef, useState } from 'react';
import type { RecordedSet } from '../../api/types';
import { parseSetInput, type LastPerformance } from '../../domain/sets';
import { Button } from '../../ui/Button';
import { Stepper } from '../../ui/Stepper';
import { defaultsForSet } from './defaults';
import type { CardPlan } from './ExerciseCard';

export type SheetTarget = {
  plan: CardPlan;
  index: number;
  recorded: RecordedSet | undefined;
  /**
   * draftId は、このシートを開いた1回の操作のID。新しいセットのIDになる。
   *
   * 押すたびに採番すると、連打した分だけ別のセットになる。開いたときに
   * 1回だけ決めれば、2回押しても同じID・同じ内容になり、サーバーも手元の
   * 記録も2回目を無害に吸収する（冪等）。
   */
  draftId: string;
};

type Props = {
  target: SheetTarget;
  name: string;
  last: LastPerformance | undefined;
  /** doneToday は今日その種目で記録済みのセット。初期値を決めるのに使う。 */
  doneToday: readonly RecordedSet[];
  /** bodyweight は自重を使う種目か。重量 0（何も付けない）を通す。 */
  bodyweight: boolean;
  onRecord: (values: { weight: number; reps: number; rir: number }) => void;
  onUndo: () => void;
  onClose: () => void;
};

export function RecordSheet({ target, name, last, doneToday, bodyweight, onRecord, onUndo, onClose }: Props) {
  const { plan, index, recorded } = target;
  const ref = useRef<HTMLDialogElement>(null);

  // 2セット目以降は今日の直前のセットに合わせる（defaults.ts に理由がある）。
  const initial = defaultsForSet({ plan, last, index, doneToday, recorded });
  const [weight, setWeight] = useState(initial.weight);
  const [reps, setReps] = useState(initial.reps);
  const [rir, setRir] = useState(initial.rir);
  const [warning, setWarning] = useState('');

  useEffect(() => {
    ref.current?.showModal();
  }, []);

  const submit = () => {
    const parsed = parseSetInput(weight, reps, rir, { bodyweight });
    if (!parsed.ok) {
      setWarning(parsed.warning);
      return;
    }
    onRecord(parsed.values);
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

        <Button onClick={submit}>記録する</Button>
        {recorded && (
          <Button variant="danger" onClick={onUndo}>
            この記録を取り消す
          </Button>
        )}
        <Button variant="quiet" onClick={() => ref.current?.close()}>
          閉じる
        </Button>
      </div>
    </dialog>
  );
}
