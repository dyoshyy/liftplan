import { useEffect, useRef, useState } from 'react';
import type { Exercise } from '../../api/types';
import { Button } from '../../ui/Button';
import { groupByPart } from '../../domain/parts';
import { Note } from '../../ui/Card';
import { Input } from '../../ui/Field';
import { pickableExercises } from './pickable';

type Props = {
  /** title はシートの見出し。何のために選ぶのかを言う。 */
  title: string;
  exercises: readonly Exercise[];
  /** excluded は選択肢に入れない種目。 */
  excluded: ReadonlySet<string>;
  /** selected は使う種目。null は読めていない（絞らない）。 */
  selected: readonly string[] | null;
  onPick: (exerciseId: string) => void;
  onClose: () => void;
};

// 種目を1つ選ぶシート。記録シートと同じく下から出す。
//
// 選んだら閉じる。選んだあと何をするかは呼び手が決める（ここでは記録しない）。
// 一覧の判断（何を出すか・どう絞るか）は pickable.ts、部位でのまとめ方は
// parts.ts（種目管理と同じ）にある。
export function ExercisePicker({ title, exercises, excluded, selected, onPick, onClose }: Props) {
  const ref = useRef<HTMLDialogElement>(null);
  const [query, setQuery] = useState('');

  useEffect(() => {
    ref.current?.showModal();
  }, []);

  const shown = pickableExercises(exercises, excluded, query, selected);

  return (
    <dialog ref={ref} onClose={onClose} onCancel={onClose} aria-label="種目を選ぶ">
      <div
        className="grid gap-3 px-4 pt-[18px]"
        style={{ paddingBottom: 'calc(20px + env(safe-area-inset-bottom))' }}
      >
        <span className="font-bold">{title}</span>

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
