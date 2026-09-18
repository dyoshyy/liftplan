import { useState } from 'react';
import type { RecordedSet } from '../../api/types';
import { today } from '../../domain/date';
import { newId } from '../../domain/id';
import type { Data } from '../../app/useLiftplan';
import { ExerciseCard, type CardPlan } from './ExerciseCard';
import { leftovers } from './leftovers';
import { RecordSheet, type SheetTarget } from './RecordSheet';
import { BodyWeight } from './BodyWeight';
import { Device } from '../settings/Device';
import { ProgramSettings } from './ProgramSettings';
import type { QueueItem } from '../../outbox/db';
import { Button } from '../../ui/Button';
import { Card, Note, SectionTitle } from '../../ui/Card';

type Props = {
  data: Data;
  offline: boolean;
  rejected: string[];
  enqueue: (item: QueueItem) => Promise<void>;
  onClearRejected: () => void;
  onRetry: () => void;
  onRecordLocally: (exerciseId: string, set: RecordedSet, replacing?: string) => void;
  onForgetLocally: (exerciseId: string, id: string) => void;
  /** 重点種目を変えたあとにメニューを取り直す。 */
  onReload: () => Promise<void>;
  /** トークンを消したあとに最初の設定へ戻す。 */
  onForget: () => void;
  /** onRecorded は記録が1件積まれたあとに呼ぶ。休憩タイマーを始めるのに使う。 */
  onRecorded: () => void;
};

export function Today(props: Props) {
  const { data, offline, rejected, enqueue, onClearRejected, onRetry, onRecorded } = props;
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
      {offline && (
        <Card title="つながりません">
          <Note>
            今日のメニューはサーバーが組むので、圏外では出せません。古いメニューを
            キャッシュして出すことはしていません。前回の重量が今日の重量として
            表示されると、記録そのものが壊れるためです。
          </Note>
          <Button variant="quiet" className="mt-3" onClick={onRetry}>
            もう一度つなぐ
          </Button>
        </Card>
      )}

      {rejected.length > 0 && (
        <Card title="送れなかった記録">
          <Note>
            サーバーが受け付けなかったので、送るのをやめました。同じものを送り続けると、
            あとの記録がすべて詰まるためです。必要なら入れ直してください。
          </Note>
          <ul className="mt-2.5 list-disc pl-[1.2em] text-xs leading-[1.7] text-faint">
            {rejected.map((r, i) => (
              <li key={i}>{r}</li>
            ))}
          </ul>
          <Button variant="quiet" className="mt-3" onClick={onClearRejected}>
            消す
          </Button>
        </Card>
      )}

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

      <BodyWeight enqueue={enqueue} />

      <ProgramSettings
        nameOf={nameOf}
        allExerciseIds={[...data.names.keys()].sort()}
        onChanged={props.onReload}
      />

      {/* トークンを消す手段。設定と名の付くものを2箇所に散らさないので、
          メニューの設定の隣に置く（D-127）。 */}
      <Device onForget={props.onForget} />

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
