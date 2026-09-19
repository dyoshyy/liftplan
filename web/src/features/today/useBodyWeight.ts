import { useState } from 'react';
import { today } from '../../domain/date';
import type { ConditionInput } from '../../api/types';
import type { QueueItem } from '../../outbox/db';

/** parseBodyWeight は入力を体重として読む。読めなければ null。 */
export function parseBodyWeight(raw: string): number | null {
  const kg = Number.parseFloat(raw);
  return Number.isFinite(kg) && kg > 0 ? kg : null;
}

// useBodyWeight は体重の入力と保存を持つ。
//
// **エラー文を出さない。**読めない値なら記録ボタンを押せなくする。
//
// 以前は押した時点で「体重を入れてください」を行の下に出していた。
// 出ると行が 44px から 71px に伸びて、下の種目カードが27px下がる。しかも
// 消えるのは保存に成功したときだけだったので、正しい値を入れ直しても赤い
// 文字が残り続けた。入力中ずっと叱られている状態になる。
//
// 単純な数値が1つあるだけの入力で、置き場所も決まっている。押せない理由は
// 見れば分かるので、文で言う必要がない。
export function useBodyWeight(
  enqueue: (item: QueueItem) => Promise<void>,
  recorded: number | undefined,
) {
  const [value, setValue] = useState('');
  const [justSaved, setJustSaved] = useState<number | undefined>(undefined);
  const [editing, setEditing] = useState(false);

  const saved = editing ? undefined : (justSaved ?? recorded);
  const parsed = parseBodyWeight(value);

  const save = async () => {
    if (parsed === null) return;
    const item: ConditionInput = { date: today(), body_weight_kg: parsed };
    await enqueue({ path: '/api/conditions', body: { conditions: [item] } });
    setJustSaved(parsed);
    setEditing(false);
  };

  const edit = () => {
    setValue(String(saved ?? ''));
    setEditing(true);
  };

  return { value, setValue, canSave: parsed !== null, saved, save, edit };
}
