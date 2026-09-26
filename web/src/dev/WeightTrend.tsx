import { daysBetween, niceAxis, shortDate, weekTicks, type WeightSeries } from './chart';

// 宣言種目の重量の推移。種目ごとに1枚、日付を横軸に並べる。
//
// 種目ごとに縦軸を持つ。ベンチ 80kg とデッドリフト 150kg を同じ軸に
// 載せると、ベンチの上下が潰れて読めない。代わりに、軸は 0 から始めず、
// 目盛りの数字を必ず出す。
//
// 折れ線は均さない。軸の一巡（0.88 と 0.81）や、バリエーションで軽く出た
// 回が、そのまま上下に出るのが見たいもの。塗りが軸レーン、白抜きが
// それ以外のレーン。

const W = 360;
const H = 168;
const PAD = { l: 40, r: 16, t: 18, b: 24 };
const INNER_W = W - PAD.l - PAD.r;
const INNER_H = H - PAD.t - PAD.b;

const LANE_NAME = { main: '軸', variation: 'バリエーション', accessory: '補助' } as const;

export function WeightTrend({ series, from, to }: { series: WeightSeries[]; from: string; to: string }) {
  return (
    <div className="grid gap-2">
      <div className="flex flex-wrap items-center gap-x-4 gap-y-1 text-[12px] text-muted">
        <span className="inline-flex items-center gap-1.5">
          <svg width="12" height="12" aria-hidden="true">
            <circle cx="6" cy="6" r="4" className="fill-amber stroke-surface" strokeWidth="1.5" />
          </svg>
          軸
        </span>
        <span className="inline-flex items-center gap-1.5">
          <svg width="12" height="12" aria-hidden="true">
            <circle cx="6" cy="6" r="4" className="fill-surface stroke-amber" strokeWidth="2" />
          </svg>
          バリエーション・補助
        </span>
        <span className="text-faint">縦軸は 0 から始まらない</span>
      </div>

      <div className="grid gap-2 sm:grid-cols-2">
        {series.map((s) => (
          <Panel key={s.id} s={s} from={from} to={to} />
        ))}
      </div>
    </div>
  );
}

function Panel({ s, from, to }: { s: WeightSeries; from: string; to: string }) {
  const axis = niceAxis(
    s.points.map((p) => p.kg),
    4,
  );

  return (
    <div className="rounded-[14px] border border-line bg-surface p-3">
      <div className="mb-1 flex items-baseline gap-2">
        <span className="text-[13px] font-bold">{s.name}</span>
        {s.undecided > 0 && (
          <span className="text-[11px] text-faint">
            重量なし {s.undecided}回（本人が決める枠）
          </span>
        )}
      </div>

      {!axis ? (
        <p className="py-6 text-center text-[12px] text-muted">重量の付いた処方がない</p>
      ) : (
        <Chart s={s} axis={axis} from={from} to={to} />
      )}
    </div>
  );
}

function Chart({
  s,
  axis,
  from,
  to,
}: {
  s: WeightSeries;
  axis: NonNullable<ReturnType<typeof niceAxis>>;
  from: string;
  to: string;
}) {
  const span = Math.max(1, daysBetween(from, to));
  const x = (date: string) => PAD.l + (daysBetween(from, date) / span) * INNER_W;
  const y = (kg: number) => PAD.t + (1 - (kg - axis.min) / (axis.max - axis.min)) * INNER_H;

  const first = s.points[0]!;
  const last = s.points[s.points.length - 1]!;
  const label = `${s.name}の重量の推移。${shortDate(first.date)}に${first.kg}kg、${shortDate(last.date)}に${last.kg}kg`;

  return (
    <>
      <svg viewBox={`0 0 ${W} ${H}`} role="img" aria-label={label} className="block w-full">
        {axis.ticks.map((t) => (
          <g key={t}>
            <line x1={PAD.l} x2={W - PAD.r} y1={y(t)} y2={y(t)} className="stroke-line-soft" />
            <text x={PAD.l - 6} y={y(t) + 3.5} textAnchor="end" fontSize="10" className="num fill-faint">
              {t}
            </text>
          </g>
        ))}

        {weekTicks(from, to, 6).map((t) => (
          <text
            key={t.date}
            x={x(t.date)}
            y={H - 6}
            textAnchor="middle"
            fontSize="10"
            className="num fill-faint"
          >
            {shortDate(t.date)}
          </text>
        ))}

        <polyline
          points={s.points.map((p) => `${x(p.date)},${y(p.kg)}`).join(' ')}
          fill="none"
          strokeWidth="2"
          strokeLinejoin="round"
          strokeLinecap="round"
          className="stroke-amber"
        />

        {s.points.map((p, i) => (
          <circle
            key={`${p.date}-${i}`}
            cx={x(p.date)}
            cy={y(p.kg)}
            r="4"
            strokeWidth={p.lane === 'main' ? 1.5 : 2}
            className={p.lane === 'main' ? 'fill-amber stroke-surface' : 'fill-surface stroke-amber'}
          >
            <title>{`${shortDate(p.date)} ${LANE_NAME[p.lane]} ${p.kg}kg`}</title>
          </circle>
        ))}

        {/* 全点に数字を付けると読めない。最初と最後だけ。 */}
        <text
          x={x(first.date)}
          y={y(first.kg) - 9}
          textAnchor="start"
          fontSize="10"
          className="num fill-muted"
        >
          {first.kg}
        </text>
        {s.points.length > 1 && (
          <text
            x={x(last.date)}
            y={y(last.kg) - 9}
            textAnchor="end"
            fontSize="10"
            className="num fill-muted"
          >
            {last.kg}
          </text>
        )}
      </svg>

      <details className="mt-1 text-[12px] text-muted">
        <summary className="cursor-pointer">数値を表で見る</summary>
        <table className="num mt-1 w-full border-separate border-spacing-y-0.5">
          <tbody>
            {s.points.map((p, i) => (
              <tr key={`${p.date}-${i}`}>
                <td>{shortDate(p.date)}</td>
                <td>{LANE_NAME[p.lane]}</td>
                <td className="text-right">{p.kg}kg</td>
              </tr>
            ))}
          </tbody>
        </table>
      </details>
    </>
  );
}
