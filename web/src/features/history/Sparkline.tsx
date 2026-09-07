import type { TrendPoint } from '../../api/types';

const W = 280;
const H = 44;
const PAD = 4;

// Sparkline は推定1RM の推移を1本の線にする。
//
// 2点では線が引けるだけで、推移として読めるものにならない。
// 面まで塗ると、無い情報があるように見える。
export function Sparkline({ points }: { points: TrendPoint[] }) {
  if (points.length < 3) return null;

  const vals = points.map((p) => p.kg);
  const min = Math.min(...vals);
  const max = Math.max(...vals);
  const span = max - min || 1;
  const x = (i: number) => PAD + (i * (W - PAD * 2)) / (points.length - 1);
  const y = (v: number) => H - PAD - ((v - min) / span) * (H - PAD * 2);

  const line = points
    .map((p, i) => `${i ? 'L' : 'M'}${x(i).toFixed(1)},${y(p.kg).toFixed(1)}`)
    .join(' ');
  const lastKg = points[points.length - 1]?.kg ?? 0;

  return (
    <svg
      className="col-span-full mb-1.5 h-11 w-full"
      viewBox={`0 0 ${W} ${H}`}
      preserveAspectRatio="none"
      aria-hidden="true"
    >
      <path
        d={`${line} L${x(points.length - 1).toFixed(1)},${H} L${x(0).toFixed(1)},${H} Z`}
        fill="rgba(224,168,46,.13)"
      />
      <path
        d={line}
        fill="none"
        stroke="#e0a82e"
        strokeWidth={1.8}
        strokeLinecap="round"
        strokeLinejoin="round"
        vectorEffect="non-scaling-stroke"
      />
      <circle cx={x(points.length - 1).toFixed(1)} cy={y(lastKg).toFixed(1)} r={3} fill="#e0a82e" />
    </svg>
  );
}
