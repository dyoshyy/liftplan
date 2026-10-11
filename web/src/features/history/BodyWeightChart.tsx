import type { TrendPoint } from '../../api/types';
import { label } from '../../domain/date';
import { Note } from '../../ui/Card';
import { layoutTrend, linePath } from './trend';
import { weightSummary } from './weightTrend';

type Props = {
  points: TrendPoint[];
  /** changeLabel は増減がいつからの差か（「8週間で」）。 */
  changeLabel: string;
};

// BodyWeightChart は体重の推移。推定1RMの推移と並べて、伸びが止まったのが
// 減量中だからか、疲れの溜めすぎかを本人が見分ける材料にする。
//
// 増減に色を付けない。推定1RMと違い、体重は増えるのも減るのも目的しだいで、
// 緑と橙を付けると「増えた＝良い」と読ませてしまう。点を選ぶ操作も付けない
// （推移は折れ線1本で足りる。user-stories）。
export function BodyWeightChart({ points, changeLabel }: Props) {
  const summary = weightSummary(points);
  if (!summary) return <Note>今日の画面で体重を記録すると、ここに推移が出ます</Note>;

  const layout = points.length >= 2 ? layoutTrend(points) : null;
  return (
    <div className="grid gap-2">
      <div className="grid grid-cols-[1fr_auto] items-center gap-3">
        <span className="text-sm font-medium">体重</span>
        <span className="num text-xl">
          {summary.currentKg.toFixed(1)}
          <small className="text-xs text-muted">kg</small>
          {summary.changeKg !== null && (
            <>
              <small className="ml-1.5 text-xs text-faint">{changeLabel}</small>
              <span className="num ml-1 rounded-full bg-surface-2 px-[7px] py-[2px] text-xs text-muted">
                {summary.changeKg > 0 ? '+' : ''}
                {summary.changeKg.toFixed(1)}
              </span>
            </>
          )}
        </span>
      </div>

      {layout && (
        <div className="grid gap-1">
          <div className="grid grid-cols-[auto_1fr] gap-2">
            <div className="num flex flex-col justify-between py-1 text-[11px] text-faint">
              <span>{layout.max.toFixed(1)}</span>
              <span>{layout.min.toFixed(1)}</span>
            </div>
            <div className="relative h-20 text-muted">
              <svg
                className="absolute inset-0 size-full"
                viewBox="0 0 100 100"
                preserveAspectRatio="none"
                aria-hidden="true"
              >
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
            </div>
          </div>
          <div className="num flex justify-between pl-11 text-[11px] text-faint">
            <span>{label(points[0]?.date ?? '')}</span>
            <span>{label(points[points.length - 1]?.date ?? '')}</span>
          </div>
        </div>
      )}
    </div>
  );
}
