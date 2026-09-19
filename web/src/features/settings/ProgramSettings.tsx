import { focusOptions, NO_FOCUS } from '../today/focus';
import { lockedSelected, toggleDeclared } from '../today/declared';
import { ExercisePicker } from './ExercisePicker';
import { regionLabel } from '../../domain/regions';
import { useProgramSettings } from './useProgramSettings';
import { Button } from '../../ui/Button';
import { Card, Note } from '../../ui/Card';
import { LabeledInput } from '../../ui/Field';
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
  } = useProgramSettings(onChanged);

  return (
    <>
      <Card title="週に通う回数">
      <Note className="mb-3">
        変えると週目標も回数に合わせて置き直されます。1週間に積めるセット数は
        通う回数に比例するので、片方だけ動かすと目標が実態を説明しなくなります。
      </Note>

      {program && (
        <div className="grid grid-cols-4 gap-2">
          {[1, 2, 3, 4].map((n) => (
            <Button
              key={n}
              variant={program.per_week === n ? 'primary' : 'quiet'}
              disabled={busy}
              onClick={() => void saveFrequency(n)}
            >
              週{n}
            </Button>
          ))}
        </div>
      )}

      </Card>

      <Card title="伸ばしたい種目">
      <Note className="mb-3">
        ここに入れた種目が、毎回1つずつ順に「軸」として出ます。最後にやったのが
        最も古いものが選ばれるので、数を増やすほど1種目あたりの頻度は下がります。
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

      </Card>

      <Card title="重点種目">
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

      </Card>

      <Card title="使う種目">
      <Note className="mb-3">
        ここに入れた種目だけが補助レーンの候補になります。伸ばしたい種目は
        外せません（先にそちらから外してください）。
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

      </Card>

      <Card title="週の目標セット数">
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
      </Card>
    </>
  );
}
