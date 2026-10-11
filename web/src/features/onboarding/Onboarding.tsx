import type { Exercise } from '../../api/types';
import { Button } from '../../ui/Button';
import { Card, Note } from '../../ui/Card';
import { Select } from '../../ui/Field';
import { Skeleton } from '../../ui/Skeleton';
import { ExercisePicker } from '../settings/ExercisePicker';
import { splitUnselectableReason } from '../settings/split';
import { lockedDeclared, toggleDeclared } from '../today/declared';
import { STEPS, withFrequency, type Step } from './onboarding';
import { useOnboarding } from './useOnboarding';

type Props = {
  exercises: readonly Exercise[];
  onDone: () => Promise<void>;
};

const TITLES: Record<Step, string> = {
  frequency: '週に何回通いますか',
  split: '分割を選ぶ',
  volume: '1回の量',
  declared: '伸ばしたい種目',
};

// 初回ログインの人に、通い方を決めてもらう画面。
//
// 聞くのは本人にしか答えられないこと（通う回数、ジムにいられる時間）と、
// 既定のままでよい好み（分割、伸ばしたい種目）。何kgでやるかは聞かない。
// それはアプリが決める。
export function Onboarding({ exercises, onDone }: Props) {
  const { program, presets, draft, step, note, busy, setDraft, next, back, finish, retry } =
    useOnboarding(onDone);
  const current = STEPS[step] ?? 'frequency';
  const last = step === STEPS.length - 1;

  return (
    <div className="grid gap-3.5">
      <div>
        <p className="text-xs tracking-[0.08em] text-faint">
          はじめに {step + 1} / {STEPS.length}
        </p>
        <h1 className="mt-1 text-[22px] font-bold">{TITLES[current]}</h1>
        {step === 0 && (
          <p className="mt-2 text-sm leading-[1.7] text-muted">
            liftplan は、何kgでやるかと補助種目をアプリが決めます。まず通い方を教えてください。
          </p>
        )}
      </div>

      <Card>
        {/* 読めなかったときの出口はここだけ。この画面では歯車も下のナビも
            出していないので、読み直す手段が無いと骨組みのまま止まる。 */}
        {(!draft || !program) && note ? (
          <Button variant="quiet" onClick={() => void retry()}>
            読み直す
          </Button>
        ) : !draft || !program ? (
          <Skeleton className="h-24 w-full rounded-xl" />
        ) : current === 'frequency' ? (
          <>
            <Note className="mb-3">
              迷ったら、実際に通えている回数に。あとから設定でいつでも変えられます。
            </Note>
            <Select
              aria-label="週に通う回数"
              value={draft.perWeek}
              onChange={(e) => setDraft(withFrequency(draft, Number(e.target.value)))}
            >
              {[1, 2, 3, 4, 5, 6, 7].map((n) => (
                <option key={n} value={n}>
                  週{n}回
                </option>
              ))}
            </Select>
          </>
        ) : current === 'split' ? (
          <>
            <Note className="mb-3">
              その日に補助種目が狙う部位の割り振りです。迷ったら全身法で。全身法では毎回すべての部位から選ばれます。
            </Note>
            <div className="grid gap-2">
              <Button
                variant={draft.split === null ? 'primary' : 'quiet'}
                aria-pressed={draft.split === null}
                onClick={() => setDraft({ ...draft, split: null })}
              >
                全身法
              </Button>
              {presets.map((p) => {
                const reason = splitUnselectableReason(p, draft.perWeek);
                const on = draft.split?.key === p.key;
                return (
                  <div key={p.key}>
                    <Button
                      variant={on ? 'primary' : 'quiet'}
                      aria-pressed={on}
                      disabled={reason !== null}
                      onClick={() => setDraft({ ...draft, split: p })}
                    >
                      {p.name}
                    </Button>
                    {reason && <Note className="mt-1">{reason}</Note>}
                  </div>
                );
              })}
            </div>
          </>
        ) : current === 'volume' ? (
          <>
            <Note className="mb-3">
              1回に出る種目の数と、1種目あたりのセット数です。ジムにいられる時間に合わせてください。
            </Note>
            <div className="grid grid-cols-2 gap-2">
              <Select
                aria-label="1回の種目数"
                value={draft.exercises}
                onChange={(e) => setDraft({ ...draft, exercises: Number(e.target.value) })}
              >
                {[2, 3, 4, 5, 6].map((n) => (
                  <option key={n} value={n}>
                    {n}種目
                  </option>
                ))}
              </Select>
              <Select
                aria-label="1種目あたりのセット数"
                value={draft.sets}
                onChange={(e) => setDraft({ ...draft, sets: Number(e.target.value) })}
              >
                {[2, 3, 4, 5, 6].map((n) => (
                  <option key={n} value={n}>
                    {n}セット
                  </option>
                ))}
              </Select>
            </div>
          </>
        ) : (
          <>
            <Note className="mb-3">
              重さを伸ばしていきたい種目です。毎回1種目ずつ、しばらくやっていないものから出ます。迷ったらそのままで。
            </Note>
            <ExercisePicker
              label="伸ばしたい種目"
              exercises={exercises.filter((e) => program.selected_exercises.includes(e.id))}
              chosen={draft.declared}
              lockedReason={lockedDeclared(draft.declared, program.focus_exercise)}
              disabled={busy}
              onToggle={(id) => setDraft({ ...draft, declared: toggleDeclared(draft.declared, id) })}
            />
          </>
        )}
      </Card>

      {note && (
        <p role="alert" className="text-sm text-red">
          {note}
        </p>
      )}

      <div className="grid grid-cols-2 gap-2">
        {step > 0 ? (
          <Button variant="quiet" disabled={busy} onClick={back}>
            戻る
          </Button>
        ) : (
          <span />
        )}
        <Button disabled={!draft || busy} onClick={last ? () => void finish() : next}>
          {last ? 'はじめる' : '次へ'}
        </Button>
      </div>
    </div>
  );
}
