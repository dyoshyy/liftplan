import { useEffect, useRef, useState } from 'react';
import type { Day, Exercise, StatsResponse, Volume } from '../../api/types';
import { label, today } from '../../domain/date';
import { regionLabel } from '../../domain/regions';
import { formatSets, usesBodyweight } from '../../domain/sets';
import { Button } from '../../ui/Button';
import { Card, Note } from '../../ui/Card';
import { Loading, Skeleton, SkeletonCard } from '../../ui/Skeleton';
import { Input } from '../../ui/Field';
import { cn } from '../../ui/cn';
import { ChevronLeftIcon, ChevronRightIcon } from '../../ui/icons';
import { ExercisePicker } from '../exercises/ExercisePicker';
import { monthGrid } from './calendar';
import { addableRange, exercisesOn, monthLabel, monthOf, summarize, type Month } from './month';
import { TrendRow } from './TrendChart';
import { changeWindowLabel } from './trend';
import type { MonthLogs } from './useMonthLogs';
import { EditSetSheet } from './EditSetSheet';
import { addSetTarget, type AddTarget, type EditTarget, type HistoryEditor } from './useHistoryEditor';
import { dayTonnage, formatTonnage } from './tonnage';
import { fillPercent } from './volume';
import { weeklyTotal } from './weekly';

const TOP_REGIONS = 6;

type View = 'logs' | 'stats';

/** 推移やカレンダーから開いた記録。その日へ寄せる。種目があれば開いて見せる。
 *  押すたびに新しい値になるので、同じ日をもう一度押しても寄せ直せる。 */
type Focus = { date: string; exerciseId?: string };

type Props = {
  /** 月ごとの記録。どの月を見ているかもここが持つ。 */
  logs: MonthLogs;
  /** 記録の修正・追加・削除。 */
  editor: HistoryEditor;
  /** 種目を足すときの選択肢。 */
  exercises: readonly Exercise[];
  /** selected は使う種目。null は読めていない（絞らない）。 */
  selected: readonly string[] | null;
  /** 種目ごとの、加重に足すと体重込みの負荷になる量。自重を使う種目の判別に使う。 */
  loadOffsets: Readonly<Record<string, number>>;
  /** 週の充足と推移。まだ読めていなければ null。 */
  stats: StatsResponse | null;
  /** 推移を読めなかったときの一言。空なら出さない。 */
  statsError?: string;
  /** 推移の読み直し。省略すると再読み込みのボタンを出さない。 */
  onReloadStats?: () => void;
};

// History は溜まった記録を見る画面。過去の記録を直すこともできる。
//
// 以前は読むだけの画面だった（D-127）。書き込みを足したのは、打ち間違いに
// 気づくのが翌日以降のことが多いため。今日の画面でしか直せないと、間違った
// 記録が推定1RMを汚し続ける。**書き込みの手順は今日の画面と同じもの**
// （planRecord / planUndo → 待ち行列）で、経路は増やしていない。
// 違うのは日付をその記録の日にすることだけ（useHistoryEditor）。
//
// **fetch はここに持たない。**props だけで描けるようにしてあるので、
// 呼び出し側が「いつ取るか」を決められる。取りに行くのは useStats と
// useMonthLogs、書くのは useHistoryEditor。
//
// **記録と推移は切り替えにする。**充足は「直近4週」で固定なのに、記録は
// 選んだ月を出す。1本のスクロールに並べると、8月を見ているのに上の数字は
// 今週のまま、という読み違いが起きる。
export function History({
  logs,
  editor,
  exercises,
  selected,
  loadOffsets,
  stats,
  statsError = '',
  onReloadStats,
}: Props) {
  const [view, setView] = useState<View>('logs');
  const [focus, setFocus] = useState<Focus | null>(null);
  // 種目を選んでいる日。選んだら、その日にその種目の1セット目を足すシートを開く。
  const [pickingFor, setPickingFor] = useState<string | null>(null);
  const { target } = editor;
  const nameOf = (id: string) => exercises.find((e) => e.id === id)?.name ?? id;

  // 推移の点から、その日の記録へ。月を合わせてから記録の側へ切り替える。
  const openLog = (date: string, exerciseId: string) => {
    setFocus({ date, exerciseId });
    logs.show(monthOf(date));
    setView('logs');
  };

  const pickDay = (date: string) => setFocus({ date });

  // 寄せた状態は、月を送るか推移へ戻ったら捨てる。残すと、前の月から
  // 戻ってきたときに、もう見終えた日へまた画面が動く。
  const leaveFocus =
    <A extends unknown[]>(f: (...a: A) => void) =>
    (...a: A) => {
      setFocus(null);
      f(...a);
    };

  return (
    <>
      <div className="grid grid-cols-2 gap-1 rounded-xl border border-line bg-surface p-1">
        <ViewTab label="記録" active={view === 'logs'} onClick={leaveFocus(() => setView('logs'))} />
        <ViewTab label="推移" active={view === 'stats'} onClick={leaveFocus(() => setView('stats'))} />
      </div>

      {view === 'logs' ? (
        <MonthlyLogs
          logs={{ ...logs, prev: leaveFocus(logs.prev), next: leaveFocus(logs.next) }}
          focus={focus}
          onPickDay={pickDay}
          onEdit={editor.open}
          onAddSet={editor.openAdd}
          onAddExercise={setPickingFor}
        />
      ) : (
        <Stats stats={stats} error={statsError} onReload={onReloadStats} onOpenLog={openLog} />
      )}

      {pickingFor && (
        <ExercisePicker
          title={`${label(pickingFor)}に足す種目を選ぶ`}
          exercises={exercises}
          // その日に記録がある種目は、その行の「セットを足す」から足す。
          excluded={exercisesOn(logs.days ?? [], pickingFor)}
          selected={selected}
          onPick={(id) =>
            editor.openAdd({
              kind: 'add',
              date: pickingFor,
              exerciseId: id,
              name: nameOf(id),
              index: 0,
              initial: null,
            })
          }
          onClose={() => setPickingFor(null)}
        />
      )}

      {target && (
        <EditSetSheet
          // 別のセットを開いたら入力をそのセットの値で作り直す。足すセットには
          // まだ ID が無いので、日付・種目・番号で区別する。
          key={
            target.kind === 'edit' ? target.set.id : `add-${target.date}-${target.exerciseId}-${target.index}`
          }
          target={target}
          bodyweight={usesBodyweight(loadOffsets, target.exerciseId)}
          onSubmit={(v) => void (target.kind === 'add' ? editor.add(v) : editor.save(v))}
          onDelete={() => void editor.remove()}
          onClose={editor.close}
        />
      )}
    </>
  );
}

type DayActions = {
  onEdit: (t: EditTarget) => void;
  onAddSet: (t: AddTarget) => void;
  /** onAddExercise はその日に足す種目を選ばせる。 */
  onAddExercise: (date: string) => void;
};

function ViewTab({ label, active, onClick }: { label: string; active: boolean; onClick: () => void }) {
  return (
    <button
      type="button"
      aria-pressed={active}
      onClick={onClick}
      className={cn(
        'min-h-11 rounded-lg text-sm',
        active ? 'animate-tab-pick bg-surface-2 font-semibold text-text' : 'text-muted',
      )}
    >
      {label}
    </button>
  );
}

// MonthlyLogs は推移の読み込みと切り離してある。
//
// 以前は推移（/api/stats）が読めるまで記録の一覧も出さなかった。記録は
// 起動時にもう手元にあるのに、圏外のジムで開くと「読めませんでした」だけに
// なっていた。
function MonthlyLogs({
  logs,
  focus,
  onPickDay,
  ...actions
}: {
  logs: MonthLogs;
  focus: Focus | null;
  onPickDay: (date: string) => void;
} & DayActions) {
  const { month, days, error } = logs;

  // 月を送ったら頭に戻す。一覧の末尾から前の月へ進むと、前の月の末尾
  // （＝一番古い日）から見ることになる。
  //
  // 推移から来たときは頭へ戻さない。寄せる先は DayCard が自分で決める
  // （子の効果が先に走るので、ここで戻すと寄せた位置を上書きする）。
  useEffect(() => {
    if (focus && monthOf(focus.date) === month) return;
    window.scrollTo({ top: 0 });
  }, [month, focus]);

  return (
    <>
      <Card>
        <div className="flex items-center">
          <Button variant="ghost" size="icon" aria-label="前の月" onClick={logs.prev}>
            <ChevronLeftIcon />
          </Button>
          <div className="flex-1 text-center">
            <div className="num text-[17px] font-semibold tracking-[0.04em]">{monthLabel(month)}</div>
            <div className="mt-0.5 min-h-[18px] text-xs text-faint">
              {days && days.length > 0 && <MonthSummaryLine days={days} />}
            </div>
          </div>
          <Button
            variant="ghost"
            size="icon"
            aria-label="次の月"
            disabled={!logs.canNext}
            onClick={logs.next}
          >
            <ChevronRightIcon />
          </Button>
        </div>
        {days && days.length > 0 && <MonthCalendar month={month} days={days} onPick={onPickDay} />}
        {/* 読めていない月に出すと、記録がある日を除けずに種目を選ばせることになる。 */}
        {days !== null && !error && (
          <AddRecordRow key={month} month={month} onChoose={actions.onAddExercise} />
        )}
      </Card>

      {error ? (
        <Card>
          <Note className="mb-3">{error}</Note>
          <Button variant="quiet" onClick={logs.reload}>
            もう一度読む
          </Button>
        </Card>
      ) : days === null ? (
        <Loading>
          <SkeletonCard rows={3} />
          <SkeletonCard rows={2} />
          <SkeletonCard rows={3} />
        </Loading>
      ) : days.length === 0 ? (
        <Card>
          <Note>この月の記録はありません</Note>
        </Card>
      ) : (
        days.map((d) => (
          <DayCard key={d.date} day={d} focus={focus?.date === d.date ? focus : undefined} {...actions} />
        ))
      )}

      {/* 一覧の末尾まで読んだ流れで前の月へ進めるようにする。頭まで
          戻らないと送れないと、1か月分を指で巻き戻すことになる。 */}
      {days && days.length > 0 && (
        <Button variant="quiet" onClick={logs.prev}>
          前の月を見る
        </Button>
      )}
    </>
  );
}

// AddRecordRow は記録の無い日に足す入口。記録のある日は DayCard からも足せるが、
// 記録しそびれた日にはカードが無い。
function AddRecordRow({ month, onChoose }: { month: Month; onChoose: (date: string) => void }) {
  const { min, max } = addableRange(month, today());
  const [date, setDate] = useState(max);
  const inRange = date >= min && date <= max;

  return (
    <div className="mt-3 flex items-center gap-2 border-t border-line-soft pt-3">
      <Input
        type="date"
        aria-label="記録を足す日"
        min={min}
        max={max}
        value={date}
        onChange={(e) => setDate(e.target.value)}
        className="min-w-0 flex-1"
      />
      <Button
        variant="quiet"
        size="md"
        className="shrink-0"
        disabled={!inRange}
        onClick={() => onChoose(date)}
      >
        記録を足す
      </Button>
    </div>
  );
}

function MonthSummaryLine({ days }: { days: readonly Day[] }) {
  const { sessions, sets, tonnage } = summarize(days);
  return (
    <span className="num">
      {sessions}回 ・ {sets}セット ・ {formatTonnage(tonnage)}
    </span>
  );
}

function Stats({
  stats,
  error,
  onReload,
  onOpenLog,
}: {
  stats: StatsResponse | null;
  error: string;
  onReload?: () => void;
  onOpenLog: (date: string, exerciseId: string) => void;
}) {
  if (error) {
    return (
      <Card title="推移">
        <Note className="mb-3">{error}</Note>
        {onReload && (
          <Button variant="quiet" onClick={onReload}>
            もう一度読む
          </Button>
        )}
      </Card>
    );
  }

  if (!stats) {
    return (
      <Loading>
        <Card title="充足（直近4週の週あたり）">
          <Skeleton className="mb-2 h-7 w-32" />
          <Skeleton className="mb-4 h-2 w-full rounded-full" />
          <div className="grid gap-3">
            {[0, 1, 2, 3, 4, 5].map((i) => (
              <div key={i} className="grid gap-1.5">
                <Skeleton className="h-3.5 w-24" />
                <Skeleton className="h-1.5 w-full" />
              </div>
            ))}
          </div>
        </Card>
        <Card title="推定1RM の推移">
          <Skeleton className="mb-3 h-5 w-full" />
          <Skeleton className="h-28 w-full rounded-lg" />
        </Card>
      </Loading>
    );
  }

  return (
    <>
      <Card title="充足（直近4週の週あたり）">
        {/* 合計を先に出す。区分ごとの一覧は「埋まっていない順」なので、
            そのまま出すと画面の頭に 0.0 が並び、記録していても動いて
            いないように見える。 */}
        <WeeklySummary volume={stats.weekly_volume} />
        <WeeklyVolume volume={stats.weekly_volume} />
        <Note className="mt-3">補助種目は、足りていない区分から選ばれます。</Note>
      </Card>

      <Card title="推定1RM の推移">
        {stats.trends.length === 0 ? (
          <Note>推移を出すには、同じ種目の記録が2回以上必要です</Note>
        ) : (
          <div className="grid gap-4">
            {stats.trends.map((t) => (
              <TrendRow
                key={t.exercise_id}
                trend={t}
                changeLabel={changeWindowLabel(stats.from, stats.to)}
                onOpenLog={onOpenLog}
              />
            ))}
          </div>
        )}
      </Card>
    </>
  );
}

// WeeklyVolume は21区分すべてを並べない。
//
// 埋まっていない順に並んでいるので、上から数件だけ見えれば
// 「次に何を足すか」は分かる。
// 週全体の充足。区分ごとの一覧より先に出す。
function WeeklySummary({ volume }: { volume: Volume[] }) {
  const total = weeklyTotal(volume);
  if (total.target === 0) return null;

  return (
    <div className="mb-4">
      <div className="flex items-baseline gap-2">
        <span className="num text-[28px] font-semibold leading-none">{total.done.toFixed(0)}</span>
        <span className="text-[13px] text-muted">/ {total.target.toFixed(0)} セット</span>
        <span className="num ml-auto text-[13px] text-muted">{total.pct.toFixed(0)}%</span>
      </div>
      <div className="mt-2 h-2 overflow-hidden rounded-full bg-surface-2">
        <div
          className={cn('h-full origin-left animate-grow-x', total.pct >= 100 ? 'bg-green' : 'bg-amber')}
          style={{ width: `${total.pct}%` }}
        />
      </div>
    </div>
  );
}

function WeeklyVolume({ volume }: { volume: Volume[] }) {
  const [expanded, setExpanded] = useState(false);

  if (volume.length === 0) return <Note>まだ記録がありません</Note>;

  const shown = expanded ? volume : volume.slice(0, TOP_REGIONS);

  return (
    <div className="grid gap-[9px]">
      {shown.map((v) => {
        const pct = fillPercent(v.done_sets, v.target_sets);
        return (
          <div key={v.region} className="grid grid-cols-[1fr_auto] items-center gap-2 text-[13px]">
            <span className="text-muted">{regionLabel(v.region)}</span>
            <span className="num text-[13px] text-muted">
              {v.done_sets.toFixed(1)} / {v.target_sets.toFixed(1)}
            </span>
            <div className="col-span-full h-1.5 overflow-hidden rounded-[3px] bg-surface-2">
              <div
                className={cn('h-full origin-left animate-grow-x', pct >= 100 ? 'bg-green' : 'bg-amber')}
                style={{ width: `${pct}%` }}
              />
            </div>
          </div>
        );
      })}
      {volume.length > TOP_REGIONS && (
        <Button
          variant="quiet"
          size="chip"
          className="mt-2.5 justify-self-start"
          onClick={() => setExpanded(!expanded)}
        >
          {expanded ? '上位だけ表示' : `残り ${volume.length - TOP_REGIONS} 区分を表示`}
        </Button>
      )}
    </div>
  );
}

// DayCard は1日分。ふだんは種目ごとに1行で、押すとセットが1行ずつ開く。
//
// セットの行まで常に出すと、1か月で画面が倍の長さになる。見返すときに
// 要るのは「何をどれだけやったか」で、RIR や1セットずつの値は直すときに
// だけ要る。
function DayCard({
  day,
  focus,
  onEdit,
  onAddSet,
  onAddExercise,
}: {
  day: Day;
  /** この日が寄せ先なら、画面をここへ寄せる。種目があれば最初から開く。 */
  focus?: Focus;
} & Pick<DayActions, 'onEdit' | 'onAddSet' | 'onAddExercise'>) {
  const [open, setOpen] = useState<string | null>(focus?.exerciseId ?? null);
  const ref = useRef<HTMLDivElement>(null);

  // 描かれたときに1回だけ寄せる。月を取りに行く間は日がまだ無いので、
  // 日が現れた時点で効く。
  useEffect(() => {
    if (focus) ref.current?.scrollIntoView({ block: 'center' });
  }, [focus]);

  return (
    <Card ref={ref} className={cn(focus && 'border-amber/60')}>
      <div className="mb-1 flex items-baseline gap-2.5">
        <span className="num text-[15px] tracking-[0.04em]">{label(day.date)}</span>
        <span className="ml-auto text-xs text-faint">
          {day.exercises.length}種目 {day.total_sets}セット ・ {formatTonnage(dayTonnage(day))}
        </span>
      </div>
      <div className="grid">
        {day.exercises.map((e) => {
          const expanded = open === e.exercise_id;
          const name = e.name || e.exercise_id;
          return (
            <div key={e.exercise_id}>
              <button
                type="button"
                aria-expanded={expanded}
                onClick={() => setOpen(expanded ? null : e.exercise_id)}
                className="grid min-h-11 w-full grid-cols-[1fr_auto_auto] items-center gap-2.5 text-left text-sm"
              >
                <span>{name}</span>
                <span className="num text-[13px] text-muted">{formatSets(e.sets)}</span>
                {/* 押せることを見せる。行が並んでいるだけだと、直せることに気づけない。 */}
                <ChevronRightIcon
                  width={14}
                  height={14}
                  className={cn('text-faint transition-transform', expanded && 'rotate-90')}
                />
              </button>
              {expanded && (
                <div className="mb-2 grid animate-unfold gap-1">
                  {e.sets.map((s, i) => (
                    <button
                      key={s.id}
                      type="button"
                      aria-label={`${name} ${i + 1}セット目を直す`}
                      onClick={() =>
                        onEdit({
                          kind: 'edit',
                          date: day.date,
                          exerciseId: e.exercise_id,
                          name,
                          index: i,
                          set: s,
                        })
                      }
                      className="grid min-h-11 grid-cols-[1.75rem_1fr_auto] items-center rounded-lg bg-surface-2 px-3 text-left text-[13px]"
                    >
                      <span className="num text-faint">{i + 1}</span>
                      <span className="num">
                        {s.weight_kg}kg × {s.reps}
                      </span>
                      <span className="num text-muted">RIR {s.rir}</span>
                    </button>
                  ))}
                  <Button
                    variant="quiet"
                    size="chip"
                    className="justify-self-start"
                    aria-label={`${name}にセットを足す`}
                    onClick={() => onAddSet(addSetTarget(day.date, e))}
                  >
                    セットを足す
                  </Button>
                </div>
              )}
            </div>
          );
        })}
      </div>
      <Button
        variant="quiet"
        size="chip"
        className="mt-1"
        aria-label={`${label(day.date)}に種目を足す`}
        onClick={() => onAddExercise(day.date)}
      >
        種目を足す
      </Button>
    </Card>
  );
}

// MonthCalendar は月の見取り図。通った日が一目で分かり、押すとその日へ寄る。
//
// 記録の一覧は新しい日から縦に並ぶだけで、「先月は週に何回通ったか」は
// 全部読まないと分からない。濃さはセット数で変える（多い日ほど濃い）。
function MonthCalendar({
  month,
  days,
  onPick,
}: {
  month: string;
  days: readonly Day[];
  onPick: (date: string) => void;
}) {
  const grid = monthGrid(month, days);
  const todayIso = today();
  const most = Math.max(1, ...days.map((d) => d.total_sets));

  return (
    <div className="mt-3 grid gap-1" role="grid" aria-label={`${monthLabel(month)}のカレンダー`}>
      <div className="grid grid-cols-7 text-center text-[11px] text-faint">
        {'日月火水木金土'.split('').map((w) => (
          <span key={w}>{w}</span>
        ))}
      </div>
      {grid.map((week, wi) => (
        <div key={wi} className="grid grid-cols-7 gap-1" role="row">
          {week.map((c, ci) =>
            c === null ? (
              <span key={ci} />
            ) : (
              <button
                key={c.date}
                type="button"
                disabled={c.sets === 0}
                aria-label={c.sets > 0 ? `${label(c.date)} ${c.sets}セット` : label(c.date)}
                onClick={() => onPick(c.date)}
                className={cn(
                  'num grid h-9 place-items-center rounded-lg text-[13px]',
                  c.sets === 0 ? 'text-faint' : 'font-semibold text-text',
                  c.date === todayIso && 'outline outline-1 outline-line',
                )}
                style={
                  c.sets > 0
                    ? {
                        backgroundColor: `color-mix(in srgb, var(--color-amber) ${25 + (c.sets / most) * 55}%, transparent)`,
                      }
                    : undefined
                }
              >
                {c.day}
              </button>
            ),
          )}
        </div>
      ))}
    </div>
  );
}
