import { focusOptions, NO_FOCUS } from '../today/focus';
import { lockedSelected, toggleDeclared } from '../today/declared';
import { ExercisePicker } from './ExercisePicker';
import { isWholeBody, scheduleSummary } from './split';
import { useProgramSettings } from './useProgramSettings';
import { Button } from '../../ui/Button';
import { Note } from '../../ui/Card';
import { Section } from '../../ui/Section';
import { Select } from '../../ui/Field';
import type { Exercise } from '../../api/types';

type Props = {
  /** 種目IDを表示名にする。 */
  nameOf: (id: string) => string;
  /** exercises は種目マスタ。使う種目の候補であり、部位ごとにまとめるのに刺激の分布が要る。 */
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
 * どの節も同じプログラムを見ているので1つの部品にしている。
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
    note,
    busy,
    locked,
    dirty,
    pickDirty,
    chooseFocus,
    saveDeclared,
    saveSelected,
    saveFrequency,
    saveVolume,
    splitOptions,
    splitKey,
    chooseSplit,
  } = useProgramSettings(onChanged);

  return (
    <>
      {/* 失敗の文言は先頭に出す。以前は全節のあとにあり、上の節で失敗しても
          画面のずっと下に出て気づけなかった。下の節（伸ばしたい種目・使う種目）は
          失敗すると保存ボタンが残るので、節ごとに出し分けるのはまだしない。 */}
      {note && <p className="mb-3 text-[13px] text-red">{note}</p>}

      {/* 回数・量・分割は「どう通うか」という1つの問いへの答えで、互いに
          縛り合う（5分割は週4回以上でしか選べない）。別々の節に畳むと、
          分割が選べない理由を読んでから回数の節を開き直すことになる。
          回数を先に置くのは、選べる分割が回数で決まるため。 */}
      <Section title="通い方" summary={program ? scheduleSummary(program) : ''} defaultOpen>
        <p className="text-[13px] font-bold">週に通う回数</p>
        <Note className="mb-3 mt-1">
          1週間に通う回数です。補助種目の量はこの回数に合わせて決まります。
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

        <p className="mt-6 text-[13px] font-bold">1回の量</p>
        <Note className="mb-3 mt-1">
          1回に出る種目の数と、1種目あたりのセット数です。ジムで取れる時間に
          合わせてください。どの部位をどれだけやるかは、この量と通う回数から
          決まります。
        </Note>

        {program && (
          <div className="grid grid-cols-2 gap-2">
            <Select
              aria-label="1回の種目数"
              value={program.exercises_per_session}
              disabled={busy}
              onChange={(e) =>
                void saveVolume(Number(e.target.value), program.sets_per_exercise)
              }
            >
              {[2, 3, 4, 5, 6].map((n) => (
                <option key={n} value={n}>
                  {n}種目
                </option>
              ))}
            </Select>
            <Select
              aria-label="1種目あたりのセット数"
              value={program.sets_per_exercise}
              disabled={busy}
              onChange={(e) =>
                void saveVolume(program.exercises_per_session, Number(e.target.value))
              }
            >
              {[2, 3, 4, 5, 6].map((n) => (
                <option key={n} value={n}>
                  {n}セット
                </option>
              ))}
            </Select>
          </div>
        )}

        <p className="mt-6 text-[13px] font-bold">分割</p>
        <Note className="mb-3 mt-1">
          その日に補助種目が狙う筋部位を決めます。通った回数で順に回るので、
          休んでも飛びません。全身法では毎回すべての部位から選ばれます。
        </Note>

        <div className="grid gap-2">
          <Button
            variant={program && isWholeBody(program) ? 'primary' : 'quiet'}
            disabled={busy}
            onClick={() => void chooseSplit(null)}
          >
            全身法
          </Button>
          {splitOptions.map(({ preset: p, reason }) => (
            <div key={p.key}>
              <Button
                variant={splitKey === p.key ? 'primary' : 'quiet'}
                disabled={busy || reason !== null}
                title={reason ?? undefined}
                onClick={() => void chooseSplit(p)}
              >
                {p.name}
              </Button>
              {reason && <Note className="mt-1">{reason}</Note>}
            </div>
          ))}
        </div>

        {program && program.splits.length > 1 && (
          <Note className="mt-3">
            {program.splits.map((s) => s.name).join(' → ')} の順に回ります。
          </Note>
        )}
      </Section>

      {/*
        使う種目 ⊇ 伸ばしたい種目 ⊇ 重点種目 の包含順に並べる。逆だと、
        まだ使っていない種目を伸ばしたいにするために一番下（使う種目）で
        追加・保存してから上へ戻る必要があった。
      */}
      <Section
        title="使う種目"
        summary={program ? `${program.selected_exercises.length}種目` : ""}
      >
        <Note className="mb-3">
          ここで選んだ種目だけが補助として出ます。
          下の「伸ばしたい種目」に入れた種目は外せません。
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

      <Section
        title="伸ばしたい種目"
        summary={program ? `${program.declared_exercises.length}種目` : ""}
      >
        <Note className="mb-3">
          毎回1種目ずつ、しばらくやっていないものから出ます。
          増やすほど1種目あたりの間隔があきます。
          候補は上の「使う種目」で保存した種目に限ります。
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
          上の「伸ばしたい種目」から選びます。選んだ種目の派生
          （ナローグリップ、テンポなど）が、軸とは別の枠で中1日以上あけて
          出ます。指定しなければバリエーションは出ません。
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
    </>
  );
}
