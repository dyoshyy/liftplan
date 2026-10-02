import type { Enqueue } from '../../outbox/db';
import { Button } from '../../ui/Button';
import { Input } from '../../ui/Field';
import { useBodyWeight } from './useBodyWeight';

// 体重は1行で置く。
//
// 1日1回の記録なので、種目カードと同じ重みを持たせない。以前はカードで
// 説明文まで付いていて、今日やる種目より目立っていた。
//
// **どの状態でも高さを変えない。**入力中・保存後で見た目は変わるが、行の
// 高さは min-h-11 のまま。以前は保存すると 71px から 44px に縮み、下の
// 種目カードが27px動いていた。読みかけのカードが動くのは、ジムで一番
// やってほしくないこと。
//
// **落とさないのは、体重が自重種目の処方と推定に使われるから。**入れ始めるのが
// 遅れるほど、それ以前の懸垂やディップスが推定から外れる（D-120）。
export function BodyWeightRow({ enqueue }: { enqueue: Enqueue }) {
  // 今日すでに入れたかはサーバーから取れない（コンディションに取得の口が
  // 無い）。画面を開き直すと入力欄に戻るが、同じ日の同じ値は冪等なので
  // 二重に入れても壊れない。取得の口を足すのはサーバー側の変更になるので、
  // 欲しくなってからにする。
  const { value, setValue, canSave, saved, save, edit } = useBodyWeight(enqueue, undefined);

  return (
    <div className="flex min-h-11 flex-1 items-center gap-2 px-1">
      {/* 見えている「体重」を label にする。押して入力欄へ移れるし、
          読み上げにも渡る。aria-label で済ませると、押しても何も起きない。 */}
      <label htmlFor="bw" className="text-[13px] text-muted">
        体重
      </label>

      {saved !== undefined ? (
        <>
          <span className="num text-[15px]">{saved}</span>
          <span className="text-[13px] text-muted">kg</span>
          <Button variant="ghost" size="chip" className="ml-auto" onClick={edit}>
            直す
          </Button>
        </>
      ) : (
        <>
          <Input
            id="bw"
            type="number"
            inputMode="decimal"
            step="0.1"
            placeholder="75.0"
            value={value}
            onChange={(e) => setValue(e.target.value)}
            className="num w-[84px] px-2 py-1.5 text-center"
          />
          <span className="text-[13px] text-muted">kg</span>
          {/* 読めない値なら押せない。文で叱らない（useBodyWeight に理由）。 */}
          <Button
            variant="quiet"
            size="chip"
            className="ml-auto"
            disabled={!canSave}
            onClick={() => void save()}
          >
            記録
          </Button>
        </>
      )}
    </div>
  );
}
