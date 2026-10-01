import { useCallback, useState } from 'react';
import type { RecordedSet } from '../../api/types';
import type { Data } from '../../app/useLiftplan';
import { ExerciseCard, type CardPlan } from './ExerciseCard';
import { PRCelebration } from './PRCelebration';
import type { PersonalRecord } from './pr';
import { belowPlan, withPicked } from './adhoc';
import { ExercisePicker } from './ExercisePicker';
import { RecordSheet } from './RecordSheet';
import { useRecordOrchestrator } from './useRecordOrchestrator';
import { BodyWeightRow } from './BodyWeightRow';
import type { QueueItem } from '../../outbox/db';
import { Button } from '../../ui/Button';
import { SectionTitle } from '../../ui/Card';

type Props = {
  data: Data;
  enqueue: (item: QueueItem) => Promise<void>;
  onRecordLocally: (exerciseId: string, set: RecordedSet, replacing?: string) => void;
  onForgetLocally: (exerciseId: string, id: string) => void;
  /** 重点種目を変えたあとにメニューを取り直す。 */
  onReload: () => Promise<void>;
  /** onRecorded は記録が1件積まれたあとに呼ぶ。休憩タイマーを始めるのに使う。 */
  onRecorded: () => void;
  /** canStartRest は休憩がまだ動いていないか。手動の入口を出すかの判断に使う。 */
  canStartRest: boolean;
  /** onOpenForecast はこの先の予定を開く。 */
  onOpenForecast: () => void;
};

export function Today(props: Props) {
  const { data, enqueue, onRecorded } = props;

  // 祝いは「いま出ているもの」だけを持つ。seq は再生し直すための採番。
  // 1セット目と2セット目で続けて更新したとき、同じ要素のままだと
  // アニメーションが走り直さず、2回目が無音で終わる。
  const [celebration, setCelebration] = useState<{ pr: PersonalRecord; seq: number } | null>(null);
  const celebrate = useCallback(
    (pr: PersonalRecord) => setCelebration((c) => ({ pr, seq: (c?.seq ?? 0) + 1 })),
    [],
  );
  const dismiss = useCallback(() => setCelebration(null), []);

  // 「種目を選んで記録」で選んだ種目。画面を開き直すと消えるが、記録したセットは
  // 消えない（「今日やったもの」に出る。belowPlan）。
  const [picked, setPicked] = useState<string[]>([]);
  const [picking, setPicking] = useState(false);
  const pick = useCallback((id: string) => setPicked((p) => withPicked(p, id)), []);
  const closePicker = useCallback(() => setPicking(false), []);

  const nameOf = useCallback((id: string) => data.names.get(id) ?? id, [data.names]);

  // 記録の手順はオーケストレーターが持つ。この部品は描画だけをする。
  const sheet = useRecordOrchestrator({
    enqueue,
    onRecordLocally: props.onRecordLocally,
    onForgetLocally: props.onForgetLocally,
    onRestStart: onRecorded,
    history: { days: data.days, doneToday: data.doneToday },
    nameOf,
    onPersonalRecord: celebrate,
  });

  const doneOf = (id: string) => data.doneToday.get(id) ?? [];

  const main = data.session?.main ?? [];
  const variation = data.session?.variation ?? [];
  const accessories = data.session?.accessories ?? [];

  const lanes = [main, variation, accessories];
  const { adhoc, done } = belowPlan(lanes, picked, data.doneToday);

  const card = (plan: CardPlan) => (
    <ExerciseCard
      key={`${plan.exercise_id}-${plan.finished_only ? 'done' : plan.adhoc ? 'adhoc' : 'plan'}`}
      plan={plan}
      name={nameOf(plan.exercise_id)}
      last={data.last[plan.exercise_id]}
      recorded={doneOf(plan.exercise_id)}
      onOpen={(index, recorded) => sheet.open({ plan, index, recorded })}
    />
  );

  return (
    <div className="grid gap-3.5">
      <div className="flex items-center gap-3">
        <BodyWeightRow enqueue={enqueue} />
        {/* 自動で始まるのが通常の経路なので、手動の入口は控えめに置く。 */}
        {props.canStartRest && (
          <Button variant="quiet" size="chip" className="ml-auto shrink-0" onClick={onRecorded}>
            休憩を始める
          </Button>
        )}
      </div>

      {/* レーンの見出し。強度が3段階あることは、見出しでしか分からない。
          以前は HEAVY バッジがカードに付いていたが、狙いがレーンごとの定数に
          なった時点で、種目ではなく枠の性質になった（D-126）。 */}
      {main.length > 0 && <SectionTitle>軸</SectionTitle>}
      {main.map(card)}

      {variation.length > 0 && <SectionTitle>バリエーション</SectionTitle>}
      {variation.map(card)}

      {accessories.length > 0 && <SectionTitle>補助種目</SectionTitle>}
      {accessories.map(card)}

      {adhoc.length > 0 && <SectionTitle>選んだ種目</SectionTitle>}
      {adhoc.map(card)}

      {done.length > 0 && <SectionTitle>今日やったもの</SectionTitle>}
      {done.map(card)}

      {/* 予定に無い種目をやりたい日の入口。今日どこまでやるかは本人が決める
          ので、予定の下に控えめに置く。 */}
      <Button variant="quiet" onClick={() => setPicking(true)}>
        種目を選んで記録
      </Button>

      {picking && (
        <ExercisePicker
          exercises={data.exercises}
          planned={lanes}
          selected={data.selected}
          onPick={pick}
          onClose={closePicker}
        />
      )}

      <div className="mt-1 border-t border-line-soft pt-3.5">
        <button
          type="button"
          className="text-[13px] text-muted underline underline-offset-2"
          onClick={props.onOpenForecast}
        >
          この先の予定を見る →
        </button>
      </div>

      {sheet.sheet && (
        <RecordSheet
          target={sheet.sheet}
          name={nameOf(sheet.sheet.plan.exercise_id)}
          last={data.last[sheet.sheet.plan.exercise_id]}
          doneToday={doneOf(sheet.sheet.plan.exercise_id)}
          onRecord={(v) => void sheet.record(v)}
          onUndo={() => void sheet.undo()}
          onClose={sheet.close}
        />
      )}

      {celebration && (
        <PRCelebration key={celebration.seq} pr={celebration.pr} onDone={dismiss} />
      )}
    </div>
  );
}
