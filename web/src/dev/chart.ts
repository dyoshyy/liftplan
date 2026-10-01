// シミュレーション画面のグラフの判断。
//
// 描画（.tsx）に書くと vitest が拾えない。どの点を打つか、どの段階の色に
// するか、どの目盛りを引くかはここに置く。色そのもの（値）は描画側にある。

import { PART_ORDER, partOf, type Part } from '../domain/parts';
import { tone, type DevResult, type DevSet, type DevWeek } from './simulate';

// ---- 重量の推移 ------------------------------------------------------

export type Lane = 'main' | 'variation' | 'accessory';

/** WeightPoint は宣言種目が出た1回。点の高さは記録した重さ。 */
export type WeightPoint = {
  date: string;
  lane: Lane;
  /** 記録した重さ（加重）。 */
  kg: number;
  reps: number;
  rir: number;
  /** 処方の重さ。履歴が無い初回は null（本人が選んだ）。 */
  prescribedKg: number | null;
  /** 処方が無く、本人が選んだ重さか。 */
  chosen: boolean;
  /** その日の模擬ユーザーの実力（加重の1RM）。 */
  athlete1rm: number;
  /** 処方の、推定1RMに対する比。推定が立たない日は null。 */
  estPct: number | null;
};

export type WeightSeries = { id: string; name: string; points: WeightPoint[] };

/** weightSeries は宣言種目ごとに、記録した重さを出た順に集める。
 *
 *  平らな折れ線にはならない。軸の一巡（0.88 と 0.81）や、同じ種目がバリエー
 *  ションや補助で軽く出た回が、そのまま上下に出る。均さないのは、見たいのが
 *  その動きだから。
 *
 *  処方の無い回（履歴の無い初回）も点にする。落とすと推移が2回目から
 *  始まり、何から始めたのかが見えない。 */
export function weightSeries(result: DevResult, declared: { id: string; name: string }[]): WeightSeries[] {
  const lanes = [
    ['main', 'main'],
    ['variation', 'variation'],
    ['accessories', 'accessory'],
  ] as const;

  return declared.map(({ id, name }) => {
    const points: WeightPoint[] = [];
    for (const day of result.days) {
      for (const [key, lane] of lanes) {
        for (const set of day[key] as DevSet[]) {
          if (set.exercise_id !== id) continue;
          points.push({
            date: day.date,
            lane,
            kg: set.performed.weight_kg,
            reps: set.performed.reps,
            rir: set.performed.rir,
            prescribedKg: set.weight_kg,
            chosen: set.weight_kg === null,
            athlete1rm: set.athlete_1rm_kg,
            estPct: set.pct_of_1rm,
          });
        }
      }
    }
    return { id, name, points };
  });
}

// ---- 筋区分のヒートマップ --------------------------------------------

/** HeatLevel は達成率の段階。0 が目標どおりで、負が不足、正が過剰。
 *
 *  ±3 は許容帯（tone が ok でない）の外。色の濃さだけに頼らず、描画側が
 *  枠を付ける。 */
export type HeatLevel = -3 | -2 | -1 | 0 | 1 | 2 | 3;

/** heatLevel は達成率を段階にする。
 *
 *  両端は tone に委ねる。境界を二重に持つと、帯を変えたときに片方だけ
 *  動いて「赤いのに枠が無い」が起きる。 */
export function heatLevel(rate: number): HeatLevel {
  const t = tone(rate);
  if (t === 'low') return -3;
  if (t === 'high') return 3;
  if (rate < 0.8) return -2;
  if (rate < 0.95) return -1;
  if (rate <= 1.05) return 0;
  if (rate <= 1.2) return 1;
  return 2;
}

export type HeatCell = {
  week: number;
  target: number;
  done: number;
  /** 目標が0のときは null。0 として塗ると、目標の無い区分が不足に見える。 */
  rate: number | null;
  level: HeatLevel | null;
};

export type HeatRow = { region: string; part: Part; cells: HeatCell[] };

/** heatRows は週ごとの充足を、区分を行・週を列の表にする。
 *
 *  並びは部位が体の上から下（PART_ORDER）、部位の中は API が返した順。
 *  開くたびに並びが変わると探せない。 */
export function heatRows(weeks: DevWeek[]): HeatRow[] {
  const seen: string[] = [];
  for (const w of weeks) {
    for (const r of w.regions) {
      if (!seen.includes(r.region)) seen.push(r.region);
    }
  }

  const rows: HeatRow[] = seen.map((region) => ({
    region,
    part: partOf(region),
    cells: weeks.map((w) => {
      const got = w.regions.find((r) => r.region === region);
      if (!got || got.target <= 0) {
        return {
          week: w.index,
          target: got?.target ?? 0,
          done: got?.done ?? 0,
          rate: null,
          level: null,
        };
      }
      const rate = got.done / got.target;
      return { week: w.index, target: got.target, done: got.done, rate, level: heatLevel(rate) };
    }),
  }));

  // sort は安定なので、部位の中は seen の順（API の順）が残る。
  const order = (p: Part) => PART_ORDER.indexOf(p);
  return rows.sort((a, b) => order(a.part) - order(b.part));
}

// ---- 軸 --------------------------------------------------------------

export type Axis = { min: number; max: number; ticks: number[] };

// 2.5kg 刻みのプレートに合う刻み。1.7 刻みの目盛りは読めない。
const STEPS = [2.5, 5, 10, 20, 25, 50, 100, 200];

/** niceAxis は値を含む、切りのよい刻みの軸を返す。値が無ければ null。
 *
 *  0 から始めない。折れ線は変化を見るもので、0 起点にすると 80→85kg の
 *  動きが潰れる。そのぶん軸の値は必ず描画側が出す。 */
export function niceAxis(values: number[], target: number): Axis | null {
  if (values.length === 0) return null;

  const lo = Math.min(...values);
  const hi = Math.max(...values);
  const step = STEPS.find((s) => s >= (hi - lo) / target) ?? STEPS[STEPS.length - 1]!;

  let min = Math.floor(lo / step) * step;
  let max = Math.ceil(hi / step) * step;
  // 値が刻みの上に乗る（点が1つ、または全部同じ）と幅が0になる。
  if (min === max) {
    min -= step;
    max += step;
  }

  const ticks: number[] = [];
  for (let t = min; t <= max + step / 2; t += step) ticks.push(t);
  return { min, max, ticks };
}

// ---- 日付の軸 --------------------------------------------------------

/** 日付（YYYY-MM-DD）を UTC の日数にする。差を取るためだけに使う。
 *
 *  ローカル時刻で引くと、夏時間をまたぐ期間で1日ずれる。 */
function dayNumber(date: string): number {
  const [y, m, d] = date.split('-').map(Number) as [number, number, number];
  return Math.round(Date.UTC(y, m - 1, d) / 86_400_000);
}

/** daysBetween は from から to までの日数。 */
export function daysBetween(from: string, to: string): number {
  return dayNumber(to) - dayNumber(from);
}

function addDays(date: string, n: number): string {
  const t = new Date((dayNumber(date) + n) * 86_400_000);
  const pad = (v: number) => String(v).padStart(2, '0');
  return `${t.getUTCFullYear()}-${pad(t.getUTCMonth() + 1)}-${pad(t.getUTCDate())}`;
}

/** weekTicks は from から7日ごとの目盛り。max を超えるときは、先頭を残して
 *  週の整数倍の間隔で間引く。 */
export function weekTicks(from: string, to: string, max: number): { date: string; offset: number }[] {
  const weeks = Math.floor(daysBetween(from, to) / 7) + 1;
  const stride = Math.max(1, Math.ceil(weeks / max));

  const ticks: { date: string; offset: number }[] = [];
  for (let w = 0; w < weeks; w += stride) {
    ticks.push({ date: addDays(from, w * 7), offset: w * 7 });
  }
  return ticks;
}

/** shortDate は目盛りの表記（月/日）。ゼロ埋めしない。 */
export function shortDate(date: string): string {
  const [, m, d] = date.split('-').map(Number);
  return `${m}/${d}`;
}
