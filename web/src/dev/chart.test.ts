import { describe, expect, it } from 'vitest';
import { daysBetween, heatLevel, heatRows, niceAxis, shortDate, weekTicks, weightSeries } from './chart';
import { tone, type DevDay, type DevResult, type DevSet, type DevWeek } from './simulate';

const set = (over: Partial<DevSet>): DevSet => ({
  exercise_id: 'bench',
  name: 'ベンチプレス',
  weight_kg: 80,
  sets: 3,
  target_rir: 1,
  target_reps: 3,
  pct_of_1rm: 0.88,
  athlete_1rm_kg: 100,
  performed: { weight_kg: 80, reps: 3, rir: 1 },
  ...over,
});

/** done は処方どおりの重さで記録した set。 */
const done = (kg: number, over: Partial<DevSet> = {}): DevSet =>
  set({ weight_kg: kg, performed: { weight_kg: kg, reps: 3, rir: 1 }, ...over });

const day = (date: string, over: Partial<DevDay>): DevDay => ({
  date,
  split: '',
  total_sets: 0,
  main: [],
  variation: [],
  accessories: [],
  ...over,
});

const settings: DevResult['settings'] = {
  declared: ['bench'],
  focus: '',
  split: '',
  frequency: 4,
  weeks: 4,
  start: '2026-01-05',
  athlete: { growth_pct_per_week: 0, first_session_pct: 70, body_weight_kg: 75 },
  weekdays: [0, 2, 4, 6],
  reps: {},
  exercises_per_session: 4,
  sets_per_exercise: 3,
};

const result = (days: DevDay[], weeks: DevWeek[] = []): DevResult => ({ settings, days, weeks });

const bench = { id: 'bench', name: 'ベンチプレス' };
const squat = { id: 'squat', name: 'スクワット' };

describe('weightSeries', () => {
  // 宣言に無い種目まで並べると、「宣言種目の重量変化」が補助の細かい動きに埋もれる。
  it('宣言した種目だけを、宣言の並びで返す。点は種目ごとに分ける', () => {
    const r = result([
      day('2026-01-05', {
        main: [done(120, { exercise_id: 'squat' })],
        accessories: [done(20, { exercise_id: 'curl' })],
      }),
    ]);
    const got = weightSeries(r, [bench, squat]);
    expect(got.map((s) => s.id)).toEqual(['bench', 'squat']);
    // 宣言に無い curl は、どの系列にも入らない。ベンチの系列にスクワットの点も入らない。
    expect(got.map((s) => s.points.map((p) => p.kg))).toEqual([[], [120]]);
  });

  it('点は日付の順に、出たレーンを持たせて並べる', () => {
    const r = result([
      day('2026-01-05', { main: [done(80)] }),
      day('2026-01-07', { variation: [done(70)] }),
      day('2026-01-09', { accessories: [done(55)] }),
    ]);
    const [s] = weightSeries(r, [bench]);
    expect(s?.points.map((p) => [p.date, p.kg, p.lane])).toEqual([
      ['2026-01-05', 80, 'main'],
      ['2026-01-07', 70, 'variation'],
      ['2026-01-09', 55, 'accessory'],
    ]);
  });

  // 点は記録した重さ。処方と記録を並べて持ち、表で両方を出す。
  it('記録と処方と実力を1点に持つ', () => {
    const r = result([
      day('2026-01-05', {
        main: [
          set({
            weight_kg: 87.5,
            pct_of_1rm: 0.88,
            athlete_1rm_kg: 101,
            performed: { weight_kg: 87.5, reps: 2, rir: 1 },
          }),
        ],
      }),
    ]);
    const [s] = weightSeries(r, [bench]);
    expect(s?.points[0]).toEqual({
      date: '2026-01-05',
      lane: 'main',
      kg: 87.5,
      reps: 2,
      rir: 1,
      prescribedKg: 87.5,
      chosen: false,
      athlete1rm: 101,
      estPct: 0.88,
    });
  });

  // 履歴が無い初回は処方が無く、本人が選んだ重さで記録する。点から
  // 落とすと、推移が2回目から始まり、何から始めたのかが見えない。
  it('処方の無い回も、本人が選んだ重さとして点にする', () => {
    const r = result([
      day('2026-01-05', {
        main: [set({ weight_kg: null, pct_of_1rm: null, performed: { weight_kg: 70, reps: 12, rir: 1 } })],
      }),
      day('2026-01-07', { main: [done(80)] }),
    ]);
    const [s] = weightSeries(r, [bench]);
    expect(s?.points.map((p) => [p.kg, p.chosen, p.prescribedKg])).toEqual([
      [70, true, null],
      [80, false, 80],
    ]);
  });

  it('一度も出ない宣言種目は、空の系列として残す', () => {
    const [s] = weightSeries(result([]), [bench]);
    expect(s).toEqual({ id: 'bench', name: 'ベンチプレス', points: [] });
  });
});

describe('heatLevel', () => {
  // 帯の外（tone が ok でない）だけが ±3。枠を付けるのはここなので、
  // tone とずれると「赤いのに枠が無い」が起きる。
  it.each([
    [0.59, -3],
    [0.6, -2],
    [0.79, -2],
    [0.8, -1],
    [0.94, -1],
    [0.95, 0],
    [1.05, 0],
    [1.06, 1],
    [1.2, 1],
    [1.21, 2],
    [1.45, 2],
    [1.46, 3],
  ])('%s は段階 %s', (value, want) => {
    expect(heatLevel(value)).toBe(want);
  });

  it('±3 は tone が ok でないときと一致する', () => {
    for (let v = 0; v <= 2; v += 0.01) {
      expect(Math.abs(heatLevel(v)) === 3).toBe(tone(v) !== 'ok');
    }
  });
});

describe('heatRows', () => {
  const weeks: DevWeek[] = [
    {
      index: 1,
      regions: [
        { region: 'QUAD', target: 10, done: 10 },
        { region: 'CHEST_MID', target: 12, done: 6 },
        { region: 'LAT', target: 10, done: 20 },
        { region: 'CHEST_UPPER', target: 8, done: 8 },
      ],
    },
    {
      index: 2,
      regions: [
        { region: 'QUAD', target: 10, done: 11 },
        { region: 'CHEST_MID', target: 12, done: 12 },
        { region: 'LAT', target: 10, done: 10 },
        { region: 'CHEST_UPPER', target: 8, done: 8 },
      ],
    },
  ];

  // 開くたびに並びが変わると探せない。部位は体の上から下（PART_ORDER）、
  // 部位の中は API が返した順。
  it('部位の順に並べ、部位の中は返ってきた順を保つ', () => {
    const rows = heatRows(weeks);
    expect(rows.map((r) => r.region)).toEqual(['CHEST_MID', 'CHEST_UPPER', 'LAT', 'QUAD']);
    expect(rows.map((r) => r.part)).toEqual(['胸', '胸', '背中', '脚']);
  });

  it('セルは週の順で、達成率と段階と実数を持つ', () => {
    const chestMid = heatRows(weeks).find((r) => r.region === 'CHEST_MID');
    expect(chestMid?.cells).toEqual([
      { week: 1, target: 12, done: 6, rate: 0.5, level: -3 },
      { week: 2, target: 12, done: 12, rate: 1, level: 0 },
    ]);
  });

  // 目標0を達成率0として塗ると、目標の無い区分が「不足」の色になる。
  it('目標が0のセルは達成率も段階も持たない', () => {
    const rows = heatRows([{ index: 1, regions: [{ region: 'CALF', target: 0, done: 0 }] }]);
    expect(rows[0]?.cells[0]).toEqual({ week: 1, target: 0, done: 0, rate: null, level: null });
  });

  // 週によって区分の集合が違っても、列がずれない。
  it('その週に無い区分は、セルの位置を空けておく', () => {
    const rows = heatRows([
      { index: 1, regions: [{ region: 'CALF', target: 4, done: 4 }] },
      { index: 2, regions: [] },
    ]);
    expect(rows[0]?.cells.map((c) => c.week)).toEqual([1, 2]);
    expect(rows[0]?.cells[1]?.rate).toBeNull();
  });
});

describe('niceAxis', () => {
  // 2.5kg 刻みのプレートに合う目盛りにする。1.7 刻みの目盛りは読めない。
  it('値を含み、切りのよい刻みの目盛りを返す', () => {
    const a = niceAxis([82.5, 97.5], 4)!;
    expect(a.min).toBeLessThanOrEqual(82.5);
    expect(a.max).toBeGreaterThanOrEqual(97.5);
    expect(a.ticks[0]).toBe(a.min);
    expect(a.ticks.at(-1)).toBe(a.max);
    const steps = a.ticks.slice(1).map((t, i) => t - (a.ticks[i] ?? 0));
    expect(new Set(steps).size).toBe(1);
    expect([2.5, 5, 10, 20, 25, 50]).toContain(steps[0]);
  });

  // 値が1つ（点が1つだけ）でも、幅0で割らない。
  it('値がすべて同じでも、幅のある軸を返す', () => {
    const a = niceAxis([100], 4)!;
    expect(a.max).toBeGreaterThan(a.min);
    expect(a.min).toBeLessThanOrEqual(100);
    expect(a.max).toBeGreaterThanOrEqual(100);
  });

  it('値が無ければ null', () => {
    expect(niceAxis([], 4)).toBeNull();
  });
});

describe('daysBetween', () => {
  it('日数の差を返す', () => {
    expect(daysBetween('2026-01-05', '2026-01-12')).toBe(7);
  });

  // 月・年をまたぐ。夏時間のある時刻で引くと1日ずれるので、日付だけで数える。
  it('月と年をまたいでも数えられる', () => {
    expect(daysBetween('2025-12-30', '2026-01-02')).toBe(3);
    expect(daysBetween('2026-03-07', '2026-03-09')).toBe(2);
  });
});

describe('weekTicks', () => {
  it('開始日から7日ごとに目盛りを返す', () => {
    expect(weekTicks('2026-01-05', '2026-01-26', 6)).toEqual([
      { date: '2026-01-05', offset: 0 },
      { date: '2026-01-12', offset: 7 },
      { date: '2026-01-19', offset: 14 },
      { date: '2026-01-26', offset: 21 },
    ]);
  });

  // 12週を毎週ラベルにすると、狭い画面で文字が重なる。先頭は残して間引く。
  it('多すぎるときは、先頭を残して同じ間隔で間引く', () => {
    const got = weekTicks('2026-01-05', '2026-03-30', 6);
    expect(got.length).toBeLessThanOrEqual(6);
    expect(got[0]).toEqual({ date: '2026-01-05', offset: 0 });
    const gaps = got.slice(1).map((t, i) => t.offset - (got[i]?.offset ?? 0));
    expect(new Set(gaps).size).toBe(1);
    expect((gaps[0] ?? 0) % 7).toBe(0);
  });
});

describe('shortDate', () => {
  it('月/日で、ゼロ埋めしない', () => {
    expect(shortDate('2026-01-05')).toBe('1/5');
    expect(shortDate('2026-11-23')).toBe('11/23');
  });
});
