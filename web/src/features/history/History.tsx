import { useEffect, useState } from 'react';
import type { Day, StatsResponse, Trend, Volume } from '../../api/types';
import { label } from '../../domain/date';
import { regionLabel } from '../../domain/regions';
import { formatSets } from '../../domain/sets';
import { Button } from '../../ui/Button';
import { Card, Note } from '../../ui/Card';
import { cn } from '../../ui/cn';
import { ChevronLeftIcon, ChevronRightIcon } from '../../ui/icons';
import { monthLabel, summarize } from './month';
import { Sparkline } from './Sparkline';
import type { MonthLogs } from './useMonthLogs';
import { fillPercent } from './volume';
import { weeklyTotal } from './weekly';

const TOP_REGIONS = 6;

type View = 'logs' | 'stats';

type Props = {
  /** 月ごとの記録。どの月を見ているかもここが持つ。 */
  logs: MonthLogs;
  /** 週の充足と推移。まだ読めていなければ null。 */
  stats: StatsResponse | null;
  /** 推移を読めなかったときの一言。空なら出さない。 */
  statsError?: string;
  /** 推移の読み直し。省略すると再読み込みのボタンを出さない。 */
  onReloadStats?: () => void;
};

// History は溜まった記録を見る画面。書き込みは1つも無い。
//
// 読むだけなので待ち行列も契約ずれの心配も無い。増える経路は
// GET /api/stats と、過去の月を開いたときの GET /api/set-logs で、
// 記録が消える経路は1本も増えない（D-127）。
//
// **fetch はここに持たない。**props だけで描けるようにしてあるので、
// 呼び出し側が「いつ取るか」を決められる。取りに行くのは useStats と
// useMonthLogs。
//
// **記録と推移は切り替えにする。**充足は「直近4週」で固定なのに、記録は
// 選んだ月を出す。1本のスクロールに並べると、8月を見ているのに上の数字は
// 今週のまま、という読み違いが起きる。
export function History({ logs, stats, statsError = '', onReloadStats }: Props) {
  const [view, setView] = useState<View>('logs');

  return (
    <>
      <div className="grid grid-cols-2 gap-1 rounded-xl border border-line bg-surface p-1">
        <ViewTab label="記録" active={view === 'logs'} onClick={() => setView('logs')} />
        <ViewTab label="推移" active={view === 'stats'} onClick={() => setView('stats')} />
      </div>

      {view === 'logs' ? (
        <MonthlyLogs logs={logs} />
      ) : (
        <Stats stats={stats} error={statsError} onReload={onReloadStats} />
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
function MonthlyLogs({ logs }: { logs: MonthLogs }) {
  const { month, days, error } = logs;

  // 月を送ったら頭に戻す。一覧の末尾から前の月へ進むと、前の月の末尾
  // （＝一番古い日）から見ることになる。
  useEffect(() => {
    window.scrollTo({ top: 0 });
  }, [month]);

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
        days.map((d) => <DayCard key={d.date} day={d} />)
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
}: {
  stats: StatsResponse | null;
  error: string;
  onReload?: () => void;
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
        <Note className="mt-3">
          補助種目は、足りていない区分から選ばれます。
        </Note>
      </Card>

      <Card title="推定1RM の推移">
        {stats.trends.length === 0 ? (
          <Note>推移を出すには、同じ種目の記録が2回以上必要です</Note>
        ) : (
          <div className="grid gap-4">
            {stats.trends.map((t) => (
              <TrendRow key={t.exercise_id} trend={t} />
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
        <span className="num text-[28px] font-semibold leading-none">
          {total.done.toFixed(0)}
        </span>
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

function TrendRow({ trend }: { trend: Trend }) {
  // 1回しか記録が無いときに「0.0」と出すと、伸びていないように読める。
  // 比べる相手がまだ無いだけなので、増減は出さない。
  const showDelta = trend.points.length >= 2;
  const sign = trend.change_kg > 0 ? '+' : '';
  const tone =
    trend.change_kg > 0
      ? 'text-green bg-green/15'
      : trend.change_kg < 0
        ? 'text-amber bg-amber/15'
        : 'text-muted bg-surface-2';

  return (
    <div className="grid grid-cols-[1fr_auto] items-center gap-3">
      <span className="text-sm font-medium">{trend.name}</span>
      <span className="num text-xl">
        {trend.current_kg.toFixed(1)}
        <small className="text-xs text-muted">kg</small>
        {showDelta && (
          <span className={`num ml-1.5 rounded-full px-[7px] py-[2px] text-xs ${tone}`}>
            {sign}
            {trend.change_kg.toFixed(1)}
          </span>
        )}
      </span>
      <Sparkline points={trend.points} />
    </div>
  );
}

function DayCard({ day }: { day: Day }) {
  return (
    <Card>
      <div className="mb-2.5 flex items-baseline gap-2.5">
        <span className="num text-[15px] tracking-[0.04em]">{label(day.date)}</span>
        <span className="ml-auto text-xs text-faint">
          {day.exercises.length}種目 {day.total_sets}セット
        </span>
      </div>
      <div className="grid gap-[7px]">
        {day.exercises.map((e) => (
          <div
            key={e.exercise_id}
            className="grid grid-cols-[1fr_auto] items-center gap-2.5 text-sm"
          >
            <span>{e.name || e.exercise_id}</span>
            <span className="num text-[13px] text-muted">{formatSets(e.sets)}</span>
          </div>
        ))}
      </div>
    </Card>
  );
}
