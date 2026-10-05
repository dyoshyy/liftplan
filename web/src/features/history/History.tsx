import { useEffect, useRef, useState } from 'react';
import type { Day, StatsResponse, Volume } from '../../api/types';
import { label } from '../../domain/date';
import { regionLabel } from '../../domain/regions';
import { formatSets } from '../../domain/sets';
import { Button } from '../../ui/Button';
import { Card, Note } from '../../ui/Card';
import { cn } from '../../ui/cn';
import { ChevronLeftIcon, ChevronRightIcon } from '../../ui/icons';
import { monthLabel, monthOf, summarize } from './month';
import { TrendRow } from './TrendChart';
import type { MonthLogs } from './useMonthLogs';
import { EditSetSheet } from './EditSetSheet';
import type { EditTarget, HistoryEditor } from './useHistoryEditor';
import { fillPercent } from './volume';
import { weeklyTotal } from './weekly';

const TOP_REGIONS = 6;

type View = 'logs' | 'stats';

/** 推移から開いた記録。この日のこの種目へ寄せて、開いて見せる。 */
type Focus = { date: string; exerciseId: string };

type Props = {
  /** 月ごとの記録。どの月を見ているかもここが持つ。 */
  logs: MonthLogs;
  /** 記録の修正と削除。 */
  editor: HistoryEditor;
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
export function History({ logs, editor, stats, statsError = '', onReloadStats }: Props) {
  const [view, setView] = useState<View>('logs');
  const [focus, setFocus] = useState<Focus | null>(null);

  // 推移の点から、その日の記録へ。月を合わせてから記録の側へ切り替える。
  const openLog = (date: string, exerciseId: string) => {
    setFocus({ date, exerciseId });
    logs.show(monthOf(date));
    setView('logs');
  };

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
          onEdit={editor.open}
        />
      ) : (
        <Stats stats={stats} error={statsError} onReload={onReloadStats} onOpenLog={openLog} />
      )}

      {editor.target && (
        <EditSetSheet
          // 別のセットを開いたら入力をそのセットの値で作り直す。
          key={editor.target.set.id}
          target={editor.target}
          onSave={(v) => void editor.save(v)}
          onDelete={() => void editor.remove()}
          onClose={editor.close}
        />
      )}
    </>
  );
}

function ViewTab({ label, active, onClick }: { label: string; active: boolean; onClick: () => void }) {
  return (
    <button
      type="button"
      aria-pressed={active}
      onClick={onClick}
      className={cn(
        'min-h-11 rounded-lg text-sm',
        active ? 'bg-surface-2 font-semibold text-text' : 'text-muted',
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
  onEdit,
}: {
  logs: MonthLogs;
  focus: Focus | null;
  onEdit: (t: EditTarget) => void;
}) {
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
      </Card>

      {error ? (
        <Card>
          <Note className="mb-3">{error}</Note>
          <Button variant="quiet" onClick={logs.reload}>
            もう一度読む
          </Button>
        </Card>
      ) : days === null ? (
        <Card>
          <Note>読み込んでいます</Note>
        </Card>
      ) : days.length === 0 ? (
        <Card>
          <Note>この月の記録はありません</Note>
        </Card>
      ) : (
        days.map((d) => (
          <DayCard
            key={d.date}
            day={d}
            focusExerciseId={focus?.date === d.date ? focus.exerciseId : undefined}
            onEdit={onEdit}
          />
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

function MonthSummaryLine({ days }: { days: readonly Day[] }) {
  const { sessions, sets } = summarize(days);
  return (
    <span className="num">
      {sessions}回 ・ {sets}セット
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
      <Card title="推移">
        <Note>読み込んでいます</Note>
      </Card>
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
              <TrendRow key={t.exercise_id} trend={t} onOpenLog={onOpenLog} />
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
          className={total.pct >= 100 ? 'h-full bg-green' : 'h-full bg-amber'}
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
                className={`h-full ${pct >= 100 ? 'bg-green' : 'bg-amber'}`}
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
  focusExerciseId,
  onEdit,
}: {
  day: Day;
  /** 推移から開かれた種目。あれば最初から開き、画面をここへ寄せる。 */
  focusExerciseId?: string;
  onEdit: (t: EditTarget) => void;
}) {
  const [open, setOpen] = useState<string | null>(focusExerciseId ?? null);
  const ref = useRef<HTMLDivElement>(null);

  // 描かれたときに1回だけ寄せる。月を取りに行く間は日がまだ無いので、
  // 日が現れた時点で効く。
  useEffect(() => {
    if (focusExerciseId) ref.current?.scrollIntoView({ block: 'center' });
  }, [focusExerciseId]);

  return (
    <div ref={ref}>
      <Card className={cn(focusExerciseId && 'border-amber/60')}>
        <div className="mb-1 flex items-baseline gap-2.5">
          <span className="num text-[15px] tracking-[0.04em]">{label(day.date)}</span>
          <span className="ml-auto text-xs text-faint">
            {day.exercises.length}種目 {day.total_sets}セット
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
                  <div className="mb-2 grid gap-1">
                    {e.sets.map((s, i) => (
                      <button
                        key={s.id}
                        type="button"
                        aria-label={`${name} ${i + 1}セット目を直す`}
                        onClick={() =>
                          onEdit({ date: day.date, exerciseId: e.exercise_id, name, index: i, set: s })
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
                  </div>
                )}
              </div>
            );
          })}
        </div>
      </Card>
    </div>
  );
}
