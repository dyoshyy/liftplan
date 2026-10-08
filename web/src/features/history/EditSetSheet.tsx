import { useEffect, useRef, useState } from 'react';
import { label } from '../../domain/date';
import { parseSetInput, type SetValues } from '../../domain/sets';
import { Button } from '../../ui/Button';
import { Stepper } from '../../ui/Stepper';
import type { SheetTarget } from './useHistoryEditor';

type Props = {
  target: SheetTarget;
  /** bodyweight は自重を使う種目か。重量 0（何も付けない）を通す。 */
  bodyweight: boolean;
  onSubmit: (values: SetValues) => void;
  /** onDelete は直すときだけ出す。足すときは消す相手がまだ無い。 */
  onDelete: () => void;
  onClose: () => void;
};

// EditSetSheet は履歴の1セットを直す・足すシート。
//
// 今日の RecordSheet を使い回さないのは、あちらが「今日の計画」と
// 「前回の実績」から初期値を決めるため。ここで直すのは記録済みの値そのもの
// なので、初期値はそのセットの値で決まる。足すときは呼び手が決めた値
// （その種目の最後のセット、新しい種目なら空）。入力の検証（parseSetInput）と
// 数値の入力部品（Stepper）は共有している。
export function EditSetSheet({ target, bodyweight, onSubmit, onDelete, onClose }: Props) {
  const ref = useRef<HTMLDialogElement>(null);
  const adding = target.kind === 'add';
  const initial = adding ? target.initial : target.set;
  const [weight, setWeight] = useState(initial ? String(initial.weight_kg) : '');
  const [reps, setReps] = useState(initial ? String(initial.reps) : '');
  const [rir, setRir] = useState(initial ? String(initial.rir) : '');
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
    onSubmit(parsed.values);
  };

  return (
    <dialog ref={ref} onClose={onClose} onCancel={onClose}>
      <div
        className="grid gap-3.5 px-4 pt-[18px]"
        style={{ paddingBottom: 'calc(20px + env(safe-area-inset-bottom))' }}
      >
        <div className="flex items-baseline gap-2.5">
          <span className="font-bold">
            {target.name} {target.index + 1}セット目{adding && 'を足す'}
          </span>
          <span className="num ml-auto text-xs text-faint">{label(target.date)}</span>
        </div>

        <Stepper label="重量 (kg)" value={weight} onChange={setWeight} step={2.5} decimal />
        <Stepper label="レップ（実際にできた回数）" value={reps} onChange={setReps} step={1} />
        <Stepper label="RIR（あと何回できたか）" value={rir} onChange={setRir} step={1} />

        {warning && <p className="m-0 text-[13px] text-red">{warning}</p>}

        <Button onClick={submit}>{adding ? '足す' : '直す'}</Button>
        {!adding && (
          <Button variant="danger" onClick={onDelete}>
            このセットを消す
          </Button>
        )}
        <Button variant="quiet" onClick={() => ref.current?.close()}>
          閉じる
        </Button>
      </div>
    </dialog>
  );
}
