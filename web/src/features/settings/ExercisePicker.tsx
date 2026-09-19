import type { Exercise } from '../../api/types';
import { groupByPart } from '../../domain/parts';
import { Button } from '../../ui/Button';
import { LockIcon } from '../../ui/icons';

type Props = {
  exercises: readonly Exercise[];
  /** chosen は選ばれている種目ID。 */
  chosen: readonly string[];
  /** lockedReason は外せない種目と、その理由。理由はそのまま画面に出す。 */
  lockedReason: ReadonlyMap<string, string>;
  disabled: boolean;
  onToggle: (id: string) => void;
};

// 種目を選ぶ一覧。部位ごとにまとめる。
//
// 「伸ばしたい種目」と「使う種目」で同じものを2回書いていたので、1つにした。
// 片方だけ直すと、同じ操作なのに見た目と挙動が食い違う。
//
// **まとめる単位は21の筋区分ではなく6つの部位。**36種目を21個の見出しに
// 割ると1グループが平均2種目未満になり、一覧として読めない（domain/parts.ts）。
export function ExercisePicker({ exercises, chosen, lockedReason, disabled, onToggle }: Props) {
  return (
    <div className="grid gap-3">
      {groupByPart(exercises).map((group) => (
        <div key={group.part}>
          <p className="mb-1.5 text-xs tracking-[0.08em] text-faint">{group.part}</p>
          <div className="flex flex-wrap gap-2">
            {group.items.map((e) => {
              const on = chosen.includes(e.id);
              const why = lockedReason.get(e.id);
              return (
                <Button
                  key={e.id}
                  size="chip"
                  variant={on ? 'selected' : 'quiet'}
                  disabled={disabled || (on && why !== undefined)}
                  title={why}
                  onClick={() => onToggle(e.id)}
                >
                  {on ? '✓ ' : ''}
                  {e.name}
                  {/* 理由は title に入れてある。本文に出すと、1つだけ3行に
                      膨らんで一覧の形が崩れる。外せないことだけ印で示す。 */}
                  {why && on ? <LockIcon className="ml-1.5 inline-block align-[-1px]" /> : null}
                </Button>
              );
            })}
          </div>
        </div>
      ))}
    </div>
  );
}
