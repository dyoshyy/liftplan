import {
  daysBetween,
  niceAxis,
  shortDate,
  weekTicks,
  type Lane,
  type WeightPoint,
  type WeightSeries,
} from './chart';

// 宣言種目の重量の推移。種目ごとに1行、左にグラフ、右に表を並べる。
//
// 点は模擬ユーザーが記録した重さ。破線はその日の実力（1RM）で、点と
// 破線の間隔が「処方が実力に対してどれだけ重いか」になる。処方が実力を
// 追えていれば、破線が上がると点も上がる。
//
// 表は折りたたまない。数字はホバーに隠さず文字で出し、Claude が画面の
// 文字を読んで確かめられるようにする。PC で開く前提なので、横に並べる。
//
// 種目ごとに縦軸を持つ。ベンチ 80kg とデッドリフト 150kg を同じ軸に
// 載せると、ベンチの上下が潰れて読めない。代わりに、軸は 0 から始めず、
// 目盛りの数字を必ず出す。

const W = 520;
const H = 220;
const PAD = { l: 44, r: 16, t: 18, b: 26 };
const INNER_W = W - PAD.l - PAD.r;
const INNER_H = H - PAD.t - PAD.b;

const LANE_NAME: Record<Lane, string> = { main: '軸', variation: 'バリエーション', accessory: '補助' };

export function WeightTrend({ series, from, to }: { series: WeightSeries[]; from: string; to: string }) {
  return (
    <div className="grid gap-2">
      <div className="flex flex-wrap items-center gap-x-5 gap-y-1 text-[12px] text-muted">
        <span className="inline-flex items-center gap-1.5">
          <svg width="12" height="12" aria-hidden="true">
            <Marker cx={6} cy={6} lane="main" chosen={false} />
          </svg>
          軸
        </span>
        <span className="inline-flex items-center gap-1.5">
          <svg width="12" height="12" aria-hidden="true">
            <Marker cx={6} cy={6} lane="variation" chosen={false} />
          </svg>
          バリエーション・補助
        </span>
        <span className="inline-flex items-center gap-1.5">
          <svg width="12" height="12" aria-hidden="true">
            <Marker cx={6} cy={6} lane="main" chosen />
          </svg>
          処方なし（本人が選んだ）
        </span>
        <span className="inline-flex items-center gap-1.5">
          <svg width="22" height="12" aria-hidden="true">
            <line x1="1" x2="21" y1="6" y2="6" strokeWidth="1.5" strokeDasharray="4 3" className="stroke-muted" />
          </svg>
          実力（その日の1RM）
        </span>
        <span className="text-faint">縦軸は 0 から始まらない</span>
      </div>

      {series.map((s) => (
        <Row key={s.id} s={s} from={from} to={to} />
      ))}
    </div>
  );
}

function Row({ s, from, to }: { s: WeightSeries; from: string; to: string }) {
  const axis = niceAxis(
    s.points.flatMap((p) => [p.kg, p.athlete1rm]),
    5,
  );

  return (
    <section className="rounded-[14px] border border-line bg-surface p-3" aria-label={`${s.name}の重量`}>
      <h3 className="mb-1 text-[13px] font-bold">{s.name}</h3>
      {!axis ? (
        <p className="py-4 text-[12px] text-muted">一度も出ていない</p>
      ) : (
        // 横に並べ、幅が足りなければ表をグラフの下に回す。表の中は折り返さない
        // （「82.5kg×5 RIR1」が2行に割れると読めない）。
        <div className="flex flex-wrap items-start gap-4">
          <Chart s={s} axis={axis} from={from} to={to} />
          <PointTable points={s.points} />
        </div>
      )}
    </section>
  );
}

function Marker({ cx, cy, lane, chosen }: { cx: number; cy: number; lane: Lane; chosen: boolean }) {
  if (chosen) {
    return (
      <rect
        x={cx - 4}
        y={cy - 4}
        width="8"
        height="8"
        transform={`rotate(45 ${cx} ${cy})`}
        strokeWidth="1.5"
        className="fill-ground stroke-amber"
      />
    );
  }
  return (
    <circle
      cx={cx}
      cy={cy}
      r="4"
      strokeWidth={lane === 'main' ? 1.5 : 2}
      className={lane === 'main' ? 'fill-amber stroke-surface' : 'fill-surface stroke-amber'}
    />
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
  const path = (pick: (p: WeightPoint) => number) =>
    s.points.map((p) => `${x(p.date)},${y(pick(p))}`).join(' ');

  const first = s.points[0]!;
  const last = s.points[s.points.length - 1]!;

  return (
    <svg
      viewBox={`0 0 ${W} ${H}`}
      width={W}
      height={H}
      role="img"
      aria-label={`${s.name}の記録した重量の推移。${shortDate(first.date)} ${first.kg}kg から ${shortDate(last.date)} ${last.kg}kg。実力は ${first.athlete1rm.toFixed(1)}kg から ${last.athlete1rm.toFixed(1)}kg`}
      className="block"
    >
      {axis.ticks.map((t) => (
        <g key={t}>
          <line x1={PAD.l} x2={W - PAD.r} y1={y(t)} y2={y(t)} className="stroke-line-soft" />
          <text x={PAD.l - 6} y={y(t) + 3.5} textAnchor="end" fontSize="10" className="num fill-faint">
            {t}
          </text>
        </g>
      ))}

      {weekTicks(from, to, 12).map((t) => (
        <text key={t.date} x={x(t.date)} y={H - 7} textAnchor="middle" fontSize="10" className="num fill-faint">
          {shortDate(t.date)}
        </text>
      ))}

      <polyline
        points={path((p) => p.athlete1rm)}
        fill="none"
        strokeWidth="1.5"
        strokeDasharray="4 3"
        className="stroke-muted"
      />
      <polyline
        points={path((p) => p.kg)}
        fill="none"
        strokeWidth="2"
        strokeLinejoin="round"
        strokeLinecap="round"
        className="stroke-amber"
      />

      {s.points.map((p, i) => (
        <g key={`${p.date}-${i}`}>
          <Marker cx={x(p.date)} cy={y(p.kg)} lane={p.lane} chosen={p.chosen} />
          <title>{`${shortDate(p.date)} ${LANE_NAME[p.lane]} ${p.kg}kg×${p.reps} RIR${p.rir}`}</title>
        </g>
      ))}

      {/* 全点に数字を付けると読めない。最初と最後だけ。全点は右の表にある。 */}
      <text x={x(first.date)} y={y(first.kg) - 9} textAnchor="start" fontSize="10" className="num fill-muted">
        {first.kg}
      </text>
      {s.points.length > 1 && (
        <text x={x(last.date)} y={y(last.kg) - 9} textAnchor="end" fontSize="10" className="num fill-muted">
          {last.kg}
        </text>
      )}
    </svg>
  );
}

function PointTable({ points }: { points: WeightPoint[] }) {
  return (
    <div className="max-h-[220px] min-w-[480px] flex-1 overflow-y-auto">
      <table className="num w-full whitespace-nowrap text-[12px] [&_td]:px-1.5 [&_th]:px-1.5">
        <thead className="sticky top-0 bg-surface text-muted">
          <tr>
            <th className="text-left font-normal">日付</th>
            <th className="text-left font-normal">レーン</th>
            <th className="text-right font-normal">処方</th>
            <th className="text-right font-normal">記録</th>
            <th className="text-right font-normal">実力1RM</th>
            <th className="text-right font-normal">記録/実力</th>
            <th className="text-right font-normal">処方/推定</th>
          </tr>
        </thead>
        <tbody>
          {points.map((p, i) => (
            <tr key={`${p.date}-${i}`}>
              <td>{shortDate(p.date)}</td>
              <td>{LANE_NAME[p.lane]}</td>
              <td className="text-right">{p.prescribedKg === null ? '本人が選ぶ' : `${p.prescribedKg}kg`}</td>
              <td className="text-right">
                {p.kg}kg×{p.reps} RIR{p.rir}
              </td>
              <td className="text-right">{p.athlete1rm.toFixed(1)}kg</td>
              <td className="text-right">{p.athlete1rm > 0 ? `${Math.round((p.kg / p.athlete1rm) * 100)}%` : '–'}</td>
              <td className="text-right">{p.estPct === null ? '–' : p.estPct.toFixed(2)}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}
