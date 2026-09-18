import type { PlannedSet, RecordedSet } from '../../api/types';
import { formatLast, type LastPerformance } from '../../domain/sets';

export type CardPlan = PlannedSet & {
  /** finished_only は今日やったが今の予定には入っていないもの。 */
  finished_only?: boolean;
};

type Props = {
  plan: CardPlan;
  name: string;
  last: LastPerformance | undefined;
  recorded: RecordedSet[];
  onOpen: (index: number, recorded: RecordedSet | undefined) => void;
};

export function ExerciseCard({ plan, name, last, recorded, onOpen }: Props) {
  // 予定より多く記録することはある。予定の数しか枠を出さないと、
  // はみ出したセットが画面から消えて、取り消すこともできなくなる。
  const slots = plan.finished_only ? recorded.length : Math.max(plan.sets, recorded.length);

  return (
    <div className="card grid gap-3">
      <div className="flex items-center gap-2.5">
        <span className="text-[17px] font-bold">{name}</span>
      </div>

      <Target plan={plan} />

      {/* 今日との差分は出さない。前回が標準日で今日が高強度日なら重量は
          当然変わるので、その差は「伸び」ではない。増減を要約すると
          「増えた＝良い」という誤った読み方を押し付けることになる。
          伸びているかは履歴の推定1RMの推移で見る。 */}
      {last ? (
        <div
          className="flex flex-wrap items-center gap-2 rounded-[10px] border border-line-soft
                     bg-surface-2 px-[11px] py-[9px] text-[13px] text-muted"
        >
          前回 <span className="num">{formatLast(last)}</span>
          <span className="ml-auto text-xs text-faint">{last.days_ago}日前</span>
        </div>
      ) : (
        <div className="note">前回の記録がありません</div>
      )}

      <div className="grid auto-cols-fr grid-flow-col gap-2">
        {Array.from({ length: slots }, (_, i) => {
          const rec = recorded[i];
          const extra = !plan.finished_only && i >= plan.sets;
          return (
            <button
              key={i}
              type="button"
              className={`set ${rec ? 'set-done' : ''}`}
              onClick={() => onOpen(i, rec)}
            >
              <span className="text-[11px] text-faint">
                {i + 1}セット目{extra ? '・追加' : ''}
              </span>
              <span className={`num text-[15px] ${rec ? 'text-green' : 'text-muted'}`}>
                {rec ? `${rec.weight_kg}×${rec.reps}` : '記録'}
              </span>
            </button>
          );
        })}
      </div>
    </div>
  );
}

function Target({ plan }: { plan: CardPlan }) {
  if (plan.finished_only) {
    // 予定として出すと「これからやる」ように読めるので、済んだ事実だけを出す。
    return (
      <div className="flex flex-wrap items-baseline gap-2.5">
        <span className="num text-[17px] font-medium text-muted">今日やった</span>
        <span className="text-[13px] text-muted">{plan.sets}セット</span>
      </div>
    );
  }

  return (
    <div className="flex flex-wrap items-baseline gap-2.5">
      {plan.weight_kg === null ? (
        <span className="num text-[17px] font-medium text-muted">自分で決める</span>
      ) : (
        <span className="num text-[34px] font-semibold leading-none">
          {plan.weight_kg}
          <small className="ml-[3px] text-[15px] font-normal text-muted">kg</small>
        </span>
      )}
      <span className="text-[13px] text-muted">
        {plan.sets}セット・{plan.weight_kg === null ? 'RIR' : '目標RIR'} {plan.target_rir}
      </span>
    </div>
  );
}
