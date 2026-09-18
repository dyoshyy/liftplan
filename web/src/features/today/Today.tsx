import type { RecordedSet } from '../../api/types';
import type { Data } from '../../app/useLiftplan';
import { ExerciseCard, type CardPlan } from './ExerciseCard';
import { leftovers } from './leftovers';
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
};

export function Today(props: Props) {
  const { data, enqueue, onRecorded } = props;
  // 記録の手順はオーケストレーターが持つ。この部品は描画だけをする。
  const sheet = useRecordOrchestrator({
    enqueue,
    onRecordLocally: props.onRecordLocally,
    onForgetLocally: props.onForgetLocally,
    onRestStart: onRecorded,
  });

  const nameOf = (id: string) => data.names.get(id) ?? id;
  const doneOf = (id: string) => data.doneToday.get(id) ?? [];

  const main = data.session?.main ?? [];
  const variation = data.session?.variation ?? [];
  const accessories = data.session?.accessories ?? [];

  const done = leftovers([main, variation, accessories], data.doneToday);

  const card = (plan: CardPlan) => (
    <ExerciseCard
      key={`${plan.exercise_id}-${plan.finished_only ? 'done' : 'plan'}`}
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

      {done.length > 0 && <SectionTitle>今日やったもの</SectionTitle>}
      {done.map(card)}



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
    </div>
  );
}
