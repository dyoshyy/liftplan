import { focusOptions, NO_FOCUS } from '../today/focus';
import { lockedSelected, toggleDeclared } from '../today/declared';
import { ExercisePicker } from './ExercisePicker';
import { regionLabel } from '../../domain/regions';
import { useProgramSettings } from './useProgramSettings';
import { Button } from '../../ui/Button';
import { Note } from '../../ui/Card';
import { Section } from '../../ui/Section';
import { LabeledInput, Select } from '../../ui/Field';
import type { Exercise } from '../../api/types';

type Props = {
  /** 種目IDを表示名にする。 */
  nameOf: (id: string) => string;
  /** 種目マスタ全件のID。使う種目の候補になる。 */
  /** exercises は種目マスタ。部位ごとにまとめるのに刺激の分布が要る。 */
  exercises: readonly Exercise[];
  /** 変更後にメニューを取り直す。設定はその日の献立を変える。 */
  onChanged: () => Promise<void>;
};

/**
 * メニューの組み方を変える。
 *
 * 設定画面の中身。以前は「今日」の中で畳んでいたが、ジムで見る画面に
 * 毎日は触らないものが同居していた。歯車から入る別画面へ移した。
 *
 * プログラムを取りに行くのはこの画面を開いたときだけで、毎回の読み込みには
 * 混ぜない。ジムで開くたびに要るものではない。
 *
 * 2つを1つの部品にしているのは、どちらも同じプログラムを見ているため。
 * 別々に持つと、宣言を変えたあとに重点種目の選択肢が古いままになる。
 *
 * 待ち行列を通さずその場で送る。待ち行列は記録を守るための仕組みで、
 * 設定を混ぜると圏外で押した変更がジムを出たあとに流れ、その日の
 * メニューは変わらないまま「変えたつもり」になる。失敗しても記録は
 * 1件も失わないので、その場で成否を見せるほうが正直。
 */
export function ProgramSettings({ nameOf, exercises, onChanged }: Props) {
  const {
    program,
    draft,
    setDraft,
    pick,
    setPick,
    target,
    setTarget,
    note,
    busy,
    locked,
    dirty,
    pickDirty,
    targetDirty,
    chooseFocus,
    saveDeclared,
    saveSelected,
    saveTarget,
    saveFrequency,
    presets,
    splitKey,
    chooseSplit,
  } = useProgramSettings(onChanged);

  return (
    <>
      <Section
        title="分割"
        summary={program && program.splits.length > 0 ? program.splits.map((s) => s.name).join(' → ') : "全身法"}
        defaultOpen
      >
        <Note className="mb-3">
          その日に補助種目が狙う筋部位を決めます。通った回数で順に回るので、
          休んでも飛びません。全身法では毎回すべての部位から選ばれます。
        </Note>

        <div className="grid gap-2">
          <Button
            variant={splitKey === null ? 'primary' : 'quiet'}
            disabled={busy}
            onClick={() => void chooseSplit(null)}
          >
            全身法
          </Button>
          {presets.map((p) => (
            <Button
              key={p.key}
              variant={splitKey === p.key ? 'primary' : 'quiet'}
              disabled={busy}
              onClick={() => void chooseSplit(p)}
            >
              {p.name}
            </Button>
          ))}
        </div>

        {program && program.splits.length > 1 && (
          <Note className="mt-3">
            {program.splits.map((s) => s.name).join(' → ')} の順に回ります。
          </Note>
        )}
      </Section>

      <Section title="週に通う回数" summary={program ? `週${program.per_week}回` : ""}>
      <Note className="mb-3">
        変えると週の目標セット数も一緒に変わります。
      </Note>

      {program && (
        <Select
          aria-label="週に通う回数"
          value={program.per_week}
          disabled={busy}
          onChange={(e) => void saveFrequency(Number(e.target.value))}
        >
          {[1, 2, 3, 4, 5, 6, 7].map((n) => (
            <option key={n} value={n}>
              週{n}回
            </option>
          ))}
        </Select>
      )}

      </Section>

      <Section title="伸ばしたい種目" summary={program ? `${program.declared_exercises.length}種目` : ""}>
      <Note className="mb-3">
        毎回1種目ずつ、しばらくやっていないものから出ます。
        増やすほど1種目あたりの間隔があきます。
      </Note>

      {program && draft && (
        <ExercisePicker
          exercises={exercises.filter((e) => program.selected_exercises.includes(e.id))}
          chosen={draft}
          lockedReason={locked}
          disabled={busy}
          onToggle={(id) => setDraft(toggleDeclared(draft, id))}
        />
      )}

      {dirty && (
        <Button
          className="mt-3"
          disabled={busy}
          onClick={() => void saveDeclared()}
        >
          伸ばしたい種目を保存する
        </Button>
      )}

      </Section>

      <Section title="重点種目" summary={program?.focus_exercise ? nameOf(program.focus_exercise) : "指定なし"}>
      <Note className="mb-3">
        選んだ種目の派生（ナローグリップ、テンポなど）が、軸とは別の枠で
        中1日以上あけて出ます。指定しなければバリエーションは出ません。
      </Note>

      {program && (
        <div className="flex flex-wrap gap-2">
          {focusOptions(program.declared_exercises).map((id) => {
            const chosen = (program.focus_exercise ?? NO_FOCUS) === id;
            return (
              <Button
                key={id || 'none'}
                size="chip"
                variant={chosen ? 'primary' : 'quiet'}
                disabled={busy}
                onClick={() => void chooseFocus(id)}
              >
                {id === NO_FOCUS ? '指定しない' : nameOf(id)}
              </Button>
            );
          })}
        </div>
      )}

      </Section>

      <Section title="使う種目" summary={program ? `${program.selected_exercises.length}種目` : ""}>
      <Note className="mb-3">
        ここで選んだ種目だけが補助として出ます。
        伸ばしたい種目は外せません。
      </Note>

      {program && pick && (
        <ExercisePicker
          exercises={exercises}
          chosen={pick}
          lockedReason={lockedSelected(pick, program.declared_exercises)}
          disabled={busy}
          onToggle={(id) => setPick(toggleDeclared(pick, id))}
        />
      )}

      {pickDirty && (
        <Button
          className="mt-3"
          disabled={busy}
          onClick={() => void saveSelected()}
        >
          使う種目を保存する
        </Button>
      )}

      </Section>

      <Section title="週の目標セット数" summary={target ? `${Object.keys(target).length}区分` : ""}>
      <Note className="mb-3">
        区分ごとの1週間の目安です。通う回数を変えると、ここも回数に合わせて
        置き直ります。届かない目標を置くと毎週すべてが赤字になるだけなので、
        不満が出た区分だけ動かすのが楽です。
      </Note>

      {target && (
        <div className="grid gap-2">
          {Object.keys(target)
            .sort()
            .map((region) => (
              <LabeledInput
                key={region}
                label={regionLabel(region)}
                type="number"
                inputMode="decimal"
                step="0.5"
                min="0"
                value={target[region]}
                disabled={busy}
                onChange={(e) => setTarget({ ...target, [region]: e.target.value })}
              />
            ))}
        </div>
      )}

      {targetDirty && (
        <Button
          className="mt-3"
          disabled={busy}
          onClick={() => void saveTarget()}
        >
          週の目標を保存する
        </Button>
      )}

      {note && <p className="mt-2.5 text-[13px] text-red">{note}</p>}
      </Section>
    </>
  );
}
