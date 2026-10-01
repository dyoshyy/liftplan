import { useEffect, useRef, useState } from 'react';
import type { Exercise, PlannedSet } from '../../api/types';
import { Button } from '../../ui/Button';
import { groupByPart } from '../../domain/parts';
import { Note } from '../../ui/Card';
import { Input } from '../../ui/Field';
import { pickableExercises } from './adhoc';

type Props = {
  exercises: readonly Exercise[];
  /** planned は今日の予定の3レーン。出ている種目は選択肢に入れない。 */
  planned: readonly (readonly PlannedSet[])[];
  onPick: (exerciseId: string) => void;
  onClose: () => void;
};

// 予定に無い種目を選ぶシート。記録シートと同じく下から出す。
//
// 選んだら閉じる。記録は選んだ種目のカードから始める（ここでは記録しない）。
// 一覧の判断（何を出すか・どう絞るか）は adhoc.ts、部位でのまとめ方は
// parts.ts（種目管理と同じ）にある。
export function ExercisePicker({ exercises, planned, onPick, onClose }: Props) {
  const ref = useRef<HTMLDialogElement>(null);
  const [query, setQuery] = useState('');

  useEffect(() => {
    ref.current?.showModal();
  }, []);

  const shown = pickableExercises(exercises, planned, query);

  return (
    <dialog ref={ref} onClose={onClose} onCancel={onClose} aria-label="種目を選ぶ">
      <div
        className="grid gap-3 px-4 pt-[18px]"
        style={{ paddingBottom: 'calc(20px + env(safe-area-inset-bottom))' }}
      >
        <span className="font-bold">記録する種目を選ぶ</span>

        <Input
          type="search"
          value={query}
          onChange={(e) => setQuery(e.target.value)}
          placeholder="名前で絞る"
          aria-label="種目を名前で絞る"
        />

        <div className="grid max-h-[50dvh] gap-3 overflow-y-auto">
          {groupByPart(shown).map((group) => (
            <section key={group.part} className="grid gap-2" aria-label={group.part}>
              <p className="text-xs tracking-[0.08em] text-faint">{group.part}</p>
              {group.items.map((e) => (
                <Button
                  key={e.id}
                  variant="quiet"
                  size="md"
                  className="justify-start text-left"
                  onClick={() => {
                    onPick(e.id);
                    ref.current?.close();
                  }}
                >
                  {e.name}
                </Button>
              ))}
            </section>
          ))}
          {shown.length === 0 && <Note>該当する種目がありません</Note>}
        </div>

        <Button variant="quiet" onClick={() => ref.current?.close()}>
          閉じる
        </Button>
      </div>
    </dialog>
  );
}
