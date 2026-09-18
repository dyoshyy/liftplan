import type { QueueItem } from '../../outbox/db';
import { Button } from '../../ui/Button';
import { Input } from '../../ui/Field';
import { useBodyWeight } from './useBodyWeight';

// 体重は1行で置く。
//
// 1日1回の記録なので、種目カードと同じ重みを持たせない。以前はカードで
// 説明文まで付いていて、今日やる種目より目立っていた。
//
// **落とさないのは、体重が自重種目の処方と推定に使われるから。**入れ始めるのが
// 遅れるほど、それ以前の懸垂やディップスが推定から外れる（D-120）。
export function BodyWeightRow({ enqueue }: { enqueue: (item: QueueItem) => Promise<void> }) {
  // 今日すでに入れたかはサーバーから取れない（コンディションに取得の口が
  // 無い）。画面を開き直すと入力欄に戻るが、同じ日の同じ値は冪等なので
  // 二重に入れても壊れない。取得の口を足すのはサーバー側の変更になるので、
  // 欲しくなってからにする。
  const { value, setValue, error, saved, save, edit } = useBodyWeight(enqueue, undefined);

  if (saved !== undefined) {
    return (
      <div className="flex flex-1 items-baseline gap-2 px-1 text-[13px] text-muted">
        <span>体重</span>
        <span className="num text-[15px] text-text">{saved}</span>
        <span>kg</span>
        <button type="button" className="ml-auto text-faint underline underline-offset-2" onClick={edit}>
          直す
        </button>
      </div>
    );
  }

  return (
    <div className="flex-1 px-1">
      <div className="flex items-center gap-2">
        <label htmlFor="bw" className="text-[13px] text-muted">
          体重
        </label>
        <Input
          id="bw"
          type="number"
          inputMode="decimal"
          step="0.1"
          placeholder="75.0"
          value={value}
          onChange={(e) => setValue(e.target.value)}
          className="num w-24 p-2 text-center"
        />
        <span className="text-[13px] text-muted">kg</span>
        <Button variant="quiet" size="chip" className="ml-auto" onClick={() => void save()}>
          記録
        </Button>
      </div>
      {error && <p className="mt-1.5 text-[13px] text-red">{error}</p>}
    </div>
  );
}
