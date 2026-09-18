import { useState } from 'react';
import type { RecordedSet } from '../../api/types';
import { today } from '../../domain/date';
import { newId } from '../../domain/id';
import type { Data } from '../../app/useLiftplan';
import { ExerciseCard, type CardPlan } from './ExerciseCard';
import { leftovers } from './leftovers';
import { RecordSheet, type SheetTarget } from './RecordSheet';
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
  const [sheet, setSheet] = useState<SheetTarget | null>(null);

  const nameOf = (id: string) => data.names.get(id) ?? id;
  const doneOf = (id: string) => data.doneToday.get(id) ?? [];

  const main = data.session?.main ?? [];
  const variation = data.session?.variation ?? [];
  const accessories = data.session?.accessories ?? [];

  const done = leftovers([main, variation, accessories], data.doneToday);

  const record = async (values: { weight: number; reps: number; rir: number }) => {
    if (!sheet) return;
    const { plan, recorded } = sheet;

    // 直す場合は、古い記録を消してから新しく入れる。同じIDで内容を
    // 変えるとサーバーが衝突として弾く（そういう契約にしてある）。
    if (recorded) {
      await enqueue({
        path: `/api/set-logs/${encodeURIComponent(recorded.id)}`,
        method: 'DELETE',
      });
    }

    // 直すときは同じIDを使い回す。新しいIDにすると、並び順が id 順
    // （＝作った時刻順）なので、直したセットだけが末尾に飛ぶ。
    // 1セット目を直したら3セット目になって出てくる。
    // 先に消してから入れ直すので、同一IDでも衝突にはならない。
    const id = recorded ? recorded.id : newId();
    await enqueue({
      path: '/api/set-logs',
      body: {
        logs: [
          {
            id,
            date: today(),
            exercise_id: plan.exercise_id,
            weight_kg: values.weight,
            reps: values.reps,
            rir: values.rir,
          },
        ],
      },
    });

    props.onRecordLocally(
      plan.exercise_id,
      { id, weight_kg: values.weight, reps: values.reps, rir: values.rir },
      recorded?.id,
    );

    // 新しく積んだときだけ休憩を始める。過去のセットを直しただけで
    // タイマーが走ると、いま休んでいる時間が上書きされる。
    if (!recorded) onRecorded();

    setSheet(null);
  };

  const undo = async () => {
    if (!sheet?.recorded) return;
    const { plan, recorded } = sheet;
    await enqueue({ path: `/api/set-logs/${encodeURIComponent(recorded.id)}`, method: 'DELETE' });
    props.onForgetLocally(plan.exercise_id, recorded.id);
    setSheet(null);
  };

  const card = (plan: CardPlan) => (
    <ExerciseCard
      key={`${plan.exercise_id}-${plan.finished_only ? 'done' : 'plan'}`}
      plan={plan}
      name={nameOf(plan.exercise_id)}
      last={data.last[plan.exercise_id]}
      recorded={doneOf(plan.exercise_id)}
      onOpen={(index, recorded) => setSheet({ plan, index, recorded })}
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



      {sheet && (
        <RecordSheet
          target={sheet}
          name={nameOf(sheet.plan.exercise_id)}
          last={data.last[sheet.plan.exercise_id]}
          doneToday={doneOf(sheet.plan.exercise_id)}
          onRecord={(v) => void record(v)}
          onUndo={() => void undo()}
          onClose={() => setSheet(null)}
        />
      )}
    </div>
  );
}
