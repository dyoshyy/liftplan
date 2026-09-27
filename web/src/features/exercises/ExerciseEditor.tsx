import { regionsByPart } from '../../domain/parts';
import { regionLabel } from '../../domain/regions';
import { Button } from '../../ui/Button';
import { Note } from '../../ui/Card';
import { Input, LabeledInput, Select } from '../../ui/Field';
import {
  cycleRegion,
  DEFAULT_INCREMENT_KG,
  draftProblem,
  setContribution,
  type ExerciseDraft,
} from './exerciseDraft';

type Props = {
  draft: ExerciseDraft;
  onChange: (next: ExerciseDraft) => void;
  onSave: () => void;
  onCancel: () => void;
  busy: boolean;
};

// 刻みの選択肢。プレート式（2.5kg・5kg）とケーブルスタック（1〜5kg）、
// ダンベル（1〜2kg）がどれも入る並び。
const INCREMENTS = [0.5, 1, 1.25, 2, 2.5, 5] as const;

/**
 * ExerciseEditor は種目を足す・直す共用の編集フォーム。
 *
 * 判断（送れるか・チップを押したときの次の状態・数値の反映）は
 * exerciseDraft.ts に置き、ここは下書きを描いて onChange に渡すだけ。
 * 送信そのもの（API を叩く・一覧を取り直す）は呼び手（useExerciseManager）
 * が持つ。
 *
 * 区分は21あるが、まとめずに並べると選ぶのに目が滑るので domain/parts.ts の
 * 粗い部位で見出しを分ける（ExercisePicker が種目を部位でまとめるのと同じ理由）。
 */
export function ExerciseEditor({ draft, onChange, onSave, onCancel, busy }: Props) {
  const problem = draftProblem(draft);
  // 選んだ区分は名前順で並べる。開くたびに順序が変わると、寄与を直すときに
  // 前回どこを触ったか探し直すことになる。
  const selected = Object.keys(draft.stimulus).sort();

  return (
    <div role="group" aria-label="種目の編集" className="grid gap-3 rounded-xl border border-line p-3">
      <LabeledInput
        label="名前"
        value={draft.name}
        maxLength={40}
        placeholder="アイソラテラル・ロー"
        onChange={(e) => onChange({ ...draft, name: e.target.value })}
      />

      <div>
        <p className="text-xs tracking-[0.04em] text-muted">
          効く区分（押すたびに 無し → 1.0 → 0.5 → 無し。数値は下で直せます）
        </p>
        <div className="mt-1.5 grid gap-2">
          {regionsByPart().map((g) => (
            <div key={g.part}>
              <p className="mb-1 text-xs tracking-[0.08em] text-faint">{g.part}</p>
              <div className="flex flex-wrap gap-2">
                {g.regions.map((r) => {
                  const on = draft.stimulus[r] !== undefined;
                  return (
                    <Button
                      key={r}
                      size="chip"
                      variant={on ? 'selected' : 'quiet'}
                      onClick={() => onChange(cycleRegion(draft, r))}
                    >
                      {on ? '✓ ' : ''}
                      {regionLabel(r)}
                    </Button>
                  );
                })}
              </div>
            </div>
          ))}
        </div>
      </div>

      {selected.length > 0 && (
        <div className="grid gap-2">
          <p className="text-xs tracking-[0.04em] text-muted">選んだ区分の寄与（0.1〜1.0）</p>
          {selected.map((r) => (
            <label key={r} className="flex items-center justify-between gap-2">
              <span className="text-sm">{regionLabel(r)}</span>
              <Input
                type="number"
                aria-label={`${regionLabel(r)}の寄与`}
                step={0.1}
                min={0.1}
                max={1}
                value={draft.stimulus[r]}
                className="w-24"
                onChange={(e) => onChange(setContribution(draft, r, Number(e.target.value)))}
              />
            </label>
          ))}
        </div>
      )}

      <label className="grid gap-1.5">
        <span className="text-xs tracking-[0.04em] text-muted">重さの刻み</span>
        <Select
          aria-label="重さの刻み"
          value={draft.incrementKg}
          onChange={(e) => onChange({ ...draft, incrementKg: Number(e.target.value) })}
        >
          {INCREMENTS.map((kg) => (
            <option key={kg} value={kg}>
              {kg}kg{kg === DEFAULT_INCREMENT_KG ? '（プレート式）' : ''}
            </option>
          ))}
        </Select>
      </label>

      {problem && <Note>{problem}</Note>}

      <div className="grid grid-cols-2 gap-2">
        <Button variant="quiet" disabled={busy} onClick={onCancel}>
          やめる
        </Button>
        <Button disabled={busy || problem !== null} onClick={onSave}>
          保存
        </Button>
      </div>
    </div>
  );
}
