import { focusOptions, NO_FOCUS } from '../today/focus';
import { aliveExercises } from '../exercises/exerciseDraft';
import { ExercisePicker } from './ExercisePicker';
import { REP_OPTIONS, repsRows } from './reps';
import { isWholeBody, scheduleSummary } from './split';
import { exercisesSummary, useProgramSettings } from './useProgramSettings';
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
  /** 種目マスタの編集（足す・直す・消す）は別ページで行う。 */
  onOpenExercises: () => void;
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
export function ProgramSettings({ nameOf, exercises: all, onChanged, onOpenExercises }: Props) {
  // 消した種目はサーバーから返ってくる（履歴の名前のため）が、ここでは選べない。
  const exercises = aliveExercises(all);
  const {
    program,
    note,
    busy,
    locked,
    chooseFocus,
    toggleDeclaredExercise,
    saveFrequency,
    saveVolume,
    saveReps,
    splitOptions,
    splitKey,
    chooseSplit,
  } = useProgramSettings(onChanged);
  // 軽い日のレップ数は、選んでいる重点種目の分だけ出す。
  const focusId = program?.focus_exercise ?? null;
  const focusReps = focusId ? program?.declared_reps[focusId] : undefined;

  return (
    <>
      {/* 失敗の文言はヘッダーの真下に貼り付ける。どの節も押したその場で
          保存するので、下のほうの種目を押して失敗したとき、先頭に置いただけ
          ではスクロールの外に出て気づけない。top はヘッダーの高さ（App.tsx）。 */}
      {note && (
        <p className="sticky top-[73px] z-20 -mx-4 bg-ground/95 px-4 py-2 text-[13px] text-red backdrop-blur-[10px]">
          {note}
        </p>
      )}

      {/* 回数・量・分割は「どう通うか」という1つの問いへの答えで、互いに
          縛り合う（5分割は週4回以上でしか選べない）。別々の節に畳むと、
          分割が選べない理由を読んでから回数の節を開き直すことになる。
          回数を先に置くのは、選べる分割が回数で決まるため。 */}
      <Section title="通い方" summary={program ? scheduleSummary(program) : ''} defaultOpen>
        <p className="text-[13px] font-bold">週に通う回数</p>
        <Note className="mb-3 mt-1">1週間に通う回数です。補助種目の量はこの回数に合わせて決まります。</Note>

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
          合わせてください。どの部位をどれだけやるかは、この量と通う回数から 決まります。
        </Note>

        {program && (
          <div className="grid grid-cols-2 gap-2">
            <Select
              aria-label="1回の種目数"
              value={program.exercises_per_session}
              disabled={busy}
              onChange={(e) => void saveVolume(Number(e.target.value), program.sets_per_exercise)}
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
              onChange={(e) => void saveVolume(program.exercises_per_session, Number(e.target.value))}
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
          <Note className="mt-3">{program.splits.map((s) => s.name).join(' → ')} の順に回ります。</Note>
        )}
      </Section>

      {/* 使う種目 ⊇ 伸ばしたい種目 ⊇ 重点種目 の順に並べる。上から絞り込んで
          いくので、上で足した種目がそのまま下の候補に出る。3つとも押したその場で
          保存するので、節を分けて畳む理由も無い。 */}
      <Section title="種目" summary={program ? exercisesSummary(program, nameOf) : ''}>
        <p className="text-[13px] font-bold">使う種目</p>
        <Note className="mb-3 mt-1">
          使う種目の入れ替え、効き方の調整、一覧に無い器具の追加は
          「種目を管理する」でします。使わない種目は計画にも出ません。
        </Note>
        <Button variant="quiet" disabled={busy} onClick={onOpenExercises}>
          種目を管理する
        </Button>

        <p className="mt-6 text-[13px] font-bold">伸ばしたい種目</p>
        <Note className="mb-3 mt-1">
          毎回1種目ずつ、しばらくやっていないものから出ます。 増やすほど1種目あたりの間隔があきます。
          候補は「使う種目」にした種目です。
        </Note>

        {program && (
          <ExercisePicker
            label="伸ばしたい種目"
            exercises={exercises.filter((e) => program.selected_exercises.includes(e.id))}
            chosen={program.declared_exercises}
            lockedReason={locked}
            disabled={busy}
            onToggle={(id) => void toggleDeclaredExercise(id)}
          />
        )}

        {program && repsRows(program).length > 0 && (
          <>
            <p className="mt-4 text-[13px] font-bold">重い日のレップ数</p>
            <Note className="mb-3 mt-1">
              軸に立ったときに狙う回数です。重さはこの回数から決まります。
              チンニングのように少ない回数でやらない種目は増やしてください。
            </Note>
            <div className="grid gap-2">
              {repsRows(program).map(({ id, reps }) => (
                <label key={id} className="grid grid-cols-[1fr_auto] items-center gap-3">
                  <span className="text-sm">{nameOf(id)}</span>
                  <Select
                    aria-label={`${nameOf(id)}の重い日のレップ数`}
                    value={reps.heavy}
                    disabled={busy}
                    onChange={(e) => void saveReps(id, { ...reps, heavy: Number(e.target.value) })}
                  >
                    {REP_OPTIONS.map((n) => (
                      <option key={n} value={n}>
                        {n}回
                      </option>
                    ))}
                  </Select>
                </label>
              ))}
            </div>
          </>
        )}

        <p className="mt-6 text-[13px] font-bold">重点種目</p>
        <Note className="mb-3 mt-1">
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

        {focusId && focusReps && (
          <>
            <p className="mt-4 text-[13px] font-bold">軽い日のレップ数</p>
            <Note className="mb-3 mt-1">
              重点種目は、重い日 → 軽い日 → 派生 の順に回ります。派生の日もこの回数で出ます。
            </Note>
            <Select
              aria-label={`${nameOf(focusId)}の軽い日のレップ数`}
              value={focusReps.light}
              disabled={busy}
              onChange={(e) => void saveReps(focusId, { ...focusReps, light: Number(e.target.value) })}
            >
              {REP_OPTIONS.map((n) => (
                <option key={n} value={n}>
                  {n}回
                </option>
              ))}
            </Select>
          </>
        )}
      </Section>
    </>
  );
}
