import { useState, type PointerEvent } from 'react';
import type { Trend } from '../../api/types';
import { label } from '../../domain/date';
import { Button } from '../../ui/Button';
import { ChevronLeftIcon, ChevronRightIcon } from '../../ui/icons';
import { layoutTrend, nearestIndex, stepIndex } from './trend';

type Props = {
  trend: Trend;
  /** 選んだ点の日の記録を開く。 */
  onOpenLog: (date: string, exerciseId: string) => void;
};

// TrendRow は1種目の推定1RM の推移。点を選ぶと、その日の値と記録への入口が出る。
//
// 以前は線を描くだけで、「この日の値はなぜ高い／低いのか」を確かめる手段が
// 無かった。答えは記録（重量×回数）にあるので、点から記録へ飛べるようにする。
export function TrendRow({ trend, onOpenLog }: Props) {
  const { points } = trend;
  const [picked, setPicked] = useState<number | null>(null);
  // 既定は最新の点。読み直しで点が減っても範囲に収める。
  const sel = Math.min(picked ?? points.length - 1, points.length - 1);
  const at = points[sel];
  if (!at) return null;

  const layout = layoutTrend(points);
  const prev = points[sel - 1];
  const diff = prev ? at.kg - prev.kg : null;

  // 1回しか記録が無いときに「0.0」と出すと、伸びていないように読める。
  // 比べる相手がまだ無いだけなので、増減は出さない。
  const showDelta = points.length >= 2;
  const sign = trend.change_kg > 0 ? '+' : '';
  const tone =
    trend.change_kg > 0
      ? 'text-green bg-green/15'
      : trend.change_kg < 0
        ? 'text-amber bg-amber/15'
        : 'text-muted bg-surface-2';

  const pick = (e: PointerEvent<HTMLDivElement>) => {
    const r = e.currentTarget.getBoundingClientRect();
    if (r.width === 0) return;
    setPicked(nearestIndex(layout.x, (e.clientX - r.left) / r.width));
  };

  return (
    <div className="grid gap-2">
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
      </div>

      {points.length >= 2 && (
        <div className="grid gap-1">
          <div className="grid grid-cols-[auto_1fr] gap-2">
            <div className="num flex flex-col justify-between py-1 text-[11px] text-faint">
              <span>{layout.max.toFixed(1)}</span>
              <span>{layout.min.toFixed(1)}</span>
            </div>
            {/* 点は幅が潰れる SVG の中に描かず、HTML を重ねる。横だけ伸ばす
                SVG に円を描くと楕円になる。 */}
            <div
              className="relative h-28 touch-pan-y select-none text-amber"
              onPointerDown={pick}
              onPointerMove={(e) => e.buttons > 0 && pick(e)}
            >
              <svg
                className="absolute inset-0 size-full"
                viewBox="0 0 100 100"
                preserveAspectRatio="none"
                aria-hidden="true"
              >
                <path
                  d={`${linePath(layout.x, layout.y)} L100,100 L0,100 Z`}
                  fill="currentColor"
                  fillOpacity={0.13}
                />
                <path
                  d={linePath(layout.x, layout.y)}
                  fill="none"
                  stroke="currentColor"
                  strokeWidth={1.8}
                  strokeLinecap="round"
                  strokeLinejoin="round"
                  vectorEffect="non-scaling-stroke"
                />
              </svg>
              <div
                className="absolute inset-y-0 w-px bg-current opacity-40"
                style={{ left: `${(layout.x[sel] ?? 0) * 100}%` }}
              />
              {points.map((p, i) => (
                <span
                  key={p.date}
                  className={
                    i === sel
                      ? 'absolute size-3.5 -translate-x-1/2 -translate-y-1/2 rounded-full border-2 border-surface bg-current'
                      : 'absolute size-1.5 -translate-x-1/2 -translate-y-1/2 rounded-full bg-current'
                  }
                  style={{ left: `${(layout.x[i] ?? 0) * 100}%`, top: `${(layout.y[i] ?? 0) * 100}%` }}
                />
              ))}
            </div>
          </div>
          <div className="num flex justify-between pl-11 text-[11px] text-faint">
            <span>{label(points[0]?.date ?? '')}</span>
            <span>{label(points[points.length - 1]?.date ?? '')}</span>
          </div>
        </div>
      )}

      <div className="grid grid-cols-[auto_1fr_auto] items-center gap-1 rounded-xl bg-surface-2 px-1">
        <Button
          variant="ghost"
          size="icon"
          aria-label="前の記録"
          disabled={sel === 0}
          onClick={() => setPicked(stepIndex(sel, -1, points.length))}
        >
          <ChevronLeftIcon />
        </Button>
        <div className="text-center">
          <div className="num text-[13px] text-muted">{label(at.date)}</div>
          <div className="num text-[17px] font-semibold">
            {at.kg.toFixed(1)}
            <small className="text-xs font-normal text-muted">kg</small>
            {diff !== null && diff !== 0 && (
              <span className={`ml-1.5 text-xs font-normal ${diff > 0 ? 'text-green' : 'text-amber'}`}>
                {diff > 0 ? '+' : ''}
                {diff.toFixed(1)}
              </span>
            )}
            {points.length >= 2 && sel === layout.bestIndex && (
              <span className="ml-1.5 rounded-full bg-green/15 px-[7px] py-[2px] text-xs font-normal text-green">
                最高
              </span>
            )}
          </div>
        </div>
        <Button
          variant="ghost"
          size="icon"
          aria-label="次の記録"
          disabled={sel === points.length - 1}
          onClick={() => setPicked(stepIndex(sel, 1, points.length))}
        >
          <ChevronRightIcon />
        </Button>
      </div>

      <Button
        variant="quiet"
        size="chip"
        className="justify-self-start"
        onClick={() => onOpenLog(at.date, trend.exercise_id)}
      >
        {label(at.date)} の記録を見る
      </Button>
    </div>
  );
}

const linePath = (xs: readonly number[], ys: readonly number[]) =>
  xs.map((x, i) => `${i ? 'L' : 'M'}${(x * 100).toFixed(2)},${((ys[i] ?? 0) * 100).toFixed(2)}`).join(' ');
