import type { RecordedSet } from '../../api/types';
import type { LastPerformance } from '../../domain/sets';

type Input = {
  plan: { weight_kg: number | null; target_rir: number };
  last: LastPerformance | undefined;
  /** index はカードの中での何セット目か（0 始まり）。 */
  index: number;
  /** doneToday は今日その種目で記録済みのセット。記録順に並んでいる。 */
  doneToday: readonly RecordedSet[];
  /** recorded はいま開いているセット自身の記録。直しているときだけ入る。 */
  recorded: RecordedSet | undefined;
};

export type SheetDefaults = { weight: string; reps: string; rir: string };

const DEFAULT_REPS = 8;

// defaultsForSet は記録シートの初期値を決める。
//
// **2セット目以降は、今日の直前のセットに合わせる。**
// 以前は毎回「処方の重量」と「前回のセッションの同じ番号のレップ」から
// 決めていたので、1セット目を 102.5kg でやっても2セット目には処方の
// 100kg が出た。実際にやった重さのほうが、次のセットの手がかりとして近い。
//
// 「1セット目」ではなく「直前のセット」なのは、3セット目で重量を落とした
// ときに4セット目が1セット目の重量へ戻らないようにするため。下げた判断が
// 毎回無かったことになる。
export function defaultsForSet({ plan, last, index, doneToday, recorded }: Input): SheetDefaults {
  // 直しているときは、そのセット自身の値。他の値を出すと直せない。
  if (recorded) {
    return {
      weight: String(recorded.weight_kg),
      reps: String(recorded.reps),
      rir: String(recorded.rir),
    };
  }

  const previous = doneToday[doneToday.length - 1];
  if (previous) {
    return {
      weight: String(previous.weight_kg),
      reps: String(previous.reps),
      rir: String(previous.rir),
    };
  }

  // 今日まだ何も記録していない。処方と前回から決める。
  return {
    weight: String(plan.weight_kg ?? last?.weight_kg ?? ''),
    reps: String(last?.reps[index] ?? DEFAULT_REPS),
    rir: String(plan.target_rir),
  };
}
