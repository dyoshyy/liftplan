import { useCallback, useEffect, useState } from 'react';
import { getJSON } from '../../api/client';
import type { Day, StatsResponse, Trend, Volume } from '../../api/types';
import { addDays, today } from '../../domain/date';
import { label } from '../../domain/date';
import { regionLabel } from '../../domain/regions';
import { formatSets } from '../../domain/sets';
import { Button } from '../../ui/Button';
import { Card, Note } from '../../ui/Card';
import { Sparkline } from './Sparkline';

const TOP_REGIONS = 6;

const WINDOW_DAYS = 56;

// History は溜まった記録を見る画面。書き込みは1つも無い。
//
// 読むだけなので待ち行列も契約ずれの心配も無い。増えるのは GET が
// 1本だけで、記録が消える経路は1本も増えない（D-128）。
//
// 日ごとの記録は `loadAll` が既に取っている `/api/set-logs` を使い回す。
// 充足と推移だけをこの画面が開かれたときに取りに行く。ジムで開く
// 「今日」の読み込みに、見ていない画面の往復を足さない（D-127 と同じ形）。
export function History({ days }: { days: Day[] }) {
  const [stats, setStats] = useState<StatsResponse | null>(null);
  const [error, setError] = useState('');

  const load = useCallback(async () => {
    setError('');
    const to = today();
    try {
      setStats(await getJSON<StatsResponse>(`/api/stats?from=${addDays(to, -WINDOW_DAYS)}&to=${to}`));
    } catch {
      setError('履歴を読めませんでした');
    }
  }, []);

  useEffect(() => {
    void load();
  }, [load]);

  if (error) {
    return (
      <Card title="履歴">
        <Note className="mb-3">{error}</Note>
        <Button variant="quiet" onClick={() => void load()}>
          もう一度読む
        </Button>
      </Card>
    );
  }

  if (!stats) {
    return (
      <Card title="履歴">
        <Note>読み込んでいます</Note>
      </Card>
    );
  }

  return (
    <>
      <Card title="今週の充足">
        <WeeklyVolume volume={stats.weekly_volume} />
        <Note className="mt-3">
          補助種目は、埋まっていない区分から選ばれます。今日その種目が出た理由がここにあります。
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

      {days.length === 0 ? (
        <Card>
          <Note>記録がまだありません</Note>
        </Card>
      ) : (
        days.map((d) => <DayCard key={d.date} day={d} />)
      )}
    </>
  );
}

// WeeklyVolume は21区分すべてを並べない。
//
// 埋まっていない順に並んでいるので、上から数件だけ見えれば
// 「次に何を足すか」は分かる。
function WeeklyVolume({ volume }: { volume: Volume[] }) {
  const [expanded, setExpanded] = useState(false);

  if (volume.length === 0) return <Note>まだ記録がありません</Note>;

  const shown = expanded ? volume : volume.slice(0, TOP_REGIONS);

  return (
    <div className="grid gap-[9px]">
      {shown.map((v) => {
        const pct = Math.min(100, Math.round((v.done_sets / Math.max(v.target_sets, 0.001)) * 100));
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
