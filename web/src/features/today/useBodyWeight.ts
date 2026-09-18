import { useState } from 'react';
import { today } from '../../domain/date';
import type { ConditionInput } from '../../api/types';
import type { QueueItem } from '../../outbox/db';

/** parseBodyWeight は入力を体重として読む。読めなければ理由を返す。 */
export function parseBodyWeight(raw: string): { kg: number } | { error: string } {
  const kg = Number.parseFloat(raw);
  if (!Number.isFinite(kg) || kg <= 0) return { error: '体重を入れてください' };
  return { kg };
}

// useBodyWeight は体重の入力と保存を持つ。
//
// 待ち行列に積むのは、記録と同じ扱いにするため。圏外で入れても消えない。
export function useBodyWeight(
  enqueue: (item: QueueItem) => Promise<void>,
  recorded: number | undefined,
) {
  const [value, setValue] = useState('');
  const [error, setError] = useState('');
  const [justSaved, setJustSaved] = useState<number | undefined>(undefined);
  const [editing, setEditing] = useState(false);

  const saved = editing ? undefined : (justSaved ?? recorded);

  const save = async () => {
    const parsed = parseBodyWeight(value);
    if ('error' in parsed) {
      setError(parsed.error);
      return;
    }
    setError('');
    const item: ConditionInput = { date: today(), body_weight_kg: parsed.kg };
    await enqueue({ path: '/api/conditions', body: { conditions: [item] } });
    setJustSaved(parsed.kg);
    setEditing(false);
  };

  const edit = () => {
    setValue(String(saved ?? ''));
    setEditing(true);
  };

  return { value, setValue, error, saved, save, edit };
}
