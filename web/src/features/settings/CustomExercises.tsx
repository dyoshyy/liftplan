import { useState } from 'react';
import type { Exercise } from '../../api/types';
import { regionsByPart } from '../../domain/parts';
import { regionLabel } from '../../domain/regions';
import { Button } from '../../ui/Button';
import { Note } from '../../ui/Card';
import { LabeledInput, Select } from '../../ui/Field';
import {
  cycleRole,
  DEFAULT_INCREMENT_KG,
  deleteBlockedReason,
  draftProblem,
  emptyDraft,
  type CustomExerciseDraft,
} from './customExercise';

type Props = {
  /** mine は自分が足した、まだ消していない種目。 */
  mine: readonly Exercise[];
  /** declared は伸ばしたい種目。入っている種目は消せない。 */
  declared: readonly string[];
  busy: boolean;
  onAdd: (draft: CustomExerciseDraft) => Promise<boolean>;
  onDelete: (id: string) => void;
};

// 刻みの選択肢。プレート式（2.5kg・5kg）とケーブルスタック（1〜5kg）、
// ダンベル（1〜2kg）がどれも入る並び。
const INCREMENTS = [0.5, 1, 1.25, 2, 2.5, 5] as const;

const ROLE_MARK = { primary: '主', secondary: '少し' } as const;

/**
 * 自分の種目。ジムにある器具を足し、要らなくなったら消す。
 *
 * 判断（送れるか・消せるか・本文）は customExercise.ts にあり、送るのは
 * useProgramSettings。ここは書きかけを持って描くだけ。
 *
 * 効く部位は、チップを押すたびに 無し → 主 → 少し → 無し と回す。
 * 主と少しを別の一覧に分けると、同じ部位を両方に入れる操作ができてしまう。
 */
export function CustomExercises({ mine, declared, busy, onAdd, onDelete }: Props) {
  const [draft, setDraft] = useState<CustomExerciseDraft>(emptyDraft);
  const problem = draftProblem(draft);

  return (
    <div className="grid gap-3">
      {mine.length > 0 && (
        <ul aria-label="自分の種目" className="grid gap-2">
          {mine.map((e) => {
            const why = deleteBlockedReason(declared, e.id);
            return (
              <li key={e.id} className="flex items-center justify-between gap-2">
                <span className="text-sm">{e.name}</span>
                <Button
                  size="md"
                  variant="danger"
                  disabled={busy || why !== null}
                  title={why ?? undefined}
                  aria-label={`${e.name}を消す`}
                  onClick={() => onDelete(e.id)}
                >
                  消す
                </Button>
              </li>
            );
          })}
        </ul>
      )}

      <div role="group" aria-label="種目を足す" className="grid gap-3 rounded-xl border border-line p-3">
        <LabeledInput
          label="名前"
          value={draft.name}
          maxLength={40}
          placeholder="アイソラテラル・ロー"
          onChange={(e) => setDraft({ ...draft, name: e.target.value })}
        />

        <div>
          <p className="text-xs tracking-[0.04em] text-muted">効く部位（押すたびに 主 → 少し → 外す）</p>
          <div className="mt-1.5 grid gap-2">
            {regionsByPart().map((g) => (
              <div key={g.part}>
                <p className="mb-1 text-xs tracking-[0.08em] text-faint">{g.part}</p>
                <div className="flex flex-wrap gap-2">
                  {g.regions.map((r) => {
                    const role = draft.roles[r];
                    return (
                      <Button
                        key={r}
                        size="chip"
                        variant={role === 'primary' ? 'primary' : role === 'secondary' ? 'selected' : 'quiet'}
                        aria-label={`${regionLabel(r)}${role ? `（${ROLE_MARK[role]}）` : ''}`}
                        onClick={() => setDraft(cycleRole(draft, r))}
                      >
                        {role ? `${ROLE_MARK[role]}・` : ''}
                        {regionLabel(r)}
                      </Button>
                    );
                  })}
                </div>
              </div>
            ))}
          </div>
        </div>

        <label className="grid gap-1.5">
          <span className="text-xs tracking-[0.04em] text-muted">重さの刻み</span>
          <Select
            aria-label="重さの刻み"
            value={draft.incrementKg}
            onChange={(e) => setDraft({ ...draft, incrementKg: Number(e.target.value) })}
          >
            {INCREMENTS.map((kg) => (
              <option key={kg} value={kg}>
                {kg}kg{kg === DEFAULT_INCREMENT_KG ? '（プレート式）' : ''}
              </option>
            ))}
          </Select>
        </label>

        {problem && <Note>{problem}</Note>}
        <Button
          disabled={busy || problem !== null}
          onClick={() =>
            void onAdd(draft).then((ok) => {
              if (ok) setDraft(emptyDraft());
            })
          }
        >
          足す
        </Button>
      </div>
    </div>
  );
}
