import { useEffect, useRef, useState } from 'react';
import { useBackdropClose } from '../../ui/backdrop';
import { Button } from '../../ui/Button';
import { LabeledInput } from '../../ui/Field';
import { parseRunInput, type RunValues } from './run';

type Props = {
  onRecord: (values: RunValues) => void;
  onClose: () => void;
};

// RunSheet はランの距離と時間を入れるシート。読めない値なら記録を押せない
// （文で叱らない。体重の行と同じ）。
export function RunSheet({ onRecord, onClose }: Props) {
  const ref = useRef<HTMLDialogElement>(null);
  const backdrop = useBackdropClose(ref);
  const [km, setKm] = useState('');
  const [min, setMin] = useState('');
  const [sec, setSec] = useState('');
  const parsed = parseRunInput(km, min, sec);

  useEffect(() => {
    ref.current?.showModal();
  }, []);

  return (
    <dialog ref={ref} {...backdrop} onClose={onClose} onCancel={onClose}>
      <div
        className="grid gap-3.5 px-4 pt-[18px]"
        style={{ paddingBottom: 'calc(20px + env(safe-area-inset-bottom))' }}
      >
        <span className="font-bold">ランを記録</span>

        <LabeledInput
          label="距離 (km)"
          type="number"
          inputMode="decimal"
          step="0.01"
          placeholder="5.0"
          value={km}
          onChange={(e) => setKm(e.target.value)}
          className="num"
        />
        <div className="grid grid-cols-2 gap-3">
          <LabeledInput
            label="分"
            type="number"
            inputMode="numeric"
            placeholder="30"
            value={min}
            onChange={(e) => setMin(e.target.value)}
            className="num"
          />
          <LabeledInput
            label="秒"
            type="number"
            inputMode="numeric"
            placeholder="0"
            value={sec}
            onChange={(e) => setSec(e.target.value)}
            className="num"
          />
        </div>

        <Button disabled={parsed === null} onClick={() => parsed && onRecord(parsed)}>
          記録する
        </Button>
        <Button variant="quiet" onClick={() => ref.current?.close()}>
          閉じる
        </Button>
      </div>
    </dialog>
  );
}
