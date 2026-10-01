import { describe, expect, it } from 'vitest';
import type { WeightSeries } from './chart';
import type { DevSettings, DevWeek } from './simulate';
import { liftLine, settingsLine, weekLine } from './summary';

const point = (over: Partial<WeightSeries['points'][number]>): WeightSeries['points'][number] => ({
  date: '2026-08-03',
  lane: 'main',
  kg: 80,
  reps: 3,
  rir: 1,
  prescribedKg: 80,
  chosen: false,
  athlete1rm: 100,
  estPct: 0.88,
  ...over,
});

describe('liftLine', () => {
  // 1行で「どこから始めて、どこまで行き、実力に対してどうか」が読めること。
  it('初回・最後の軸・処方の幅・実力の推移を1行にする', () => {
    const got = liftLine({
      id: 'bench',
      name: 'ベンチプレス',
      points: [
        point({ date: '2026-08-03', kg: 70, reps: 12, prescribedKg: null, chosen: true, athlete1rm: 100 }),
        point({ date: '2026-08-07', kg: 82.5, reps: 5, prescribedKg: 82.5, athlete1rm: 100.3 }),
        point({ date: '2026-08-09', lane: 'variation', kg: 75, reps: 8, prescribedKg: 75, athlete1rm: 100.4 }),
        point({ date: '2026-08-14', kg: 90, reps: 2, prescribedKg: 90, athlete1rm: 104 }),
      ],
    });
    expect(got).toBe(
      'ベンチプレス: 4回。初回 70kg×12 RIR1（本人が選んだ）。' +
        '最後の軸 8/14 90kg（実力 104kg の 87%）。処方 75〜90kg。実力 100→104kg',
    );
  });

  // 軸に一度も立たない宣言種目もありうる（分割で当たらない等）。
  it('軸に立たなければ、そう書く', () => {
    const got = liftLine({
      id: 'bench',
      name: 'ベンチプレス',
      points: [point({ lane: 'accessory', kg: 60, prescribedKg: 60 })],
    });
    expect(got).toContain('軸に立っていない');
  });

  it('一度も出なければ、そう書く', () => {
    expect(liftLine({ id: 'bench', name: 'ベンチプレス', points: [] })).toBe(
      'ベンチプレス: 一度も出ていない',
    );
  });
});

describe('weekLine', () => {
  const week: DevWeek = {
    index: 2,
    // API は区分名の昇順で返す（LAT が CHEST_* より後とは限らない）。
    // 部位の順に並べ替えていることが見えるよう、わざと背中を先に置く。
    regions: [
      { region: 'LAT', target: 10, done: 20 },
      { region: 'CHEST_MID', target: 12, done: 12 },
      { region: 'CHEST_UPPER', target: 8, done: 0 },
    ],
  };

  // 許容外だけを、名前と%で並べる。部位の並び（体の上から下）で出す。
  it('許容外の区分を名前と%で並べる', () => {
    expect(weekLine(week)).toBe('2週: 許容外 2区分 — 大胸筋上部 0%, 広背筋 200%');
  });

  it('全部収まっていれば、そう書く', () => {
    expect(weekLine({ index: 1, regions: [{ region: 'LAT', target: 10, done: 10 }] })).toBe(
      '1週: 全区分が 60〜145% に収まっている',
    );
  });

  // 目標0は「狙わない区分」で、許容外ではない（ヒートマップも空欄にしている）。
  it('目標0の区分は数えない', () => {
    expect(weekLine({ index: 1, regions: [{ region: 'CALF', target: 0, done: 0 }] })).toBe(
      '1週: 全区分が 60〜145% に収まっている',
    );
  });
});

describe('settingsLine', () => {
  const settings: DevSettings = {
    declared: ['bench', 'squat'],
    focus: 'bench',
    split: 'upper_lower',
    frequency: 4,
    weeks: 12,
    start: '2026-08-03',
    weekdays: [0, 2, 4, 6],
    exercises_per_session: 4,
    sets_per_exercise: 3,
    athlete: {
      growth_pct_per_week: 0.5,
      first_session_pct: 70,
      body_weight_kg: 75,
      one_rep_max_kg: { bench: 100, squat: 140, curl: 45 },
    },
  };
  const names = { bench: 'ベンチプレス', squat: 'スクワット' };

  // 応答が解決した設定をそのまま書く。どの仮定から出た数字かを先に読ませる。
  it('前提を1行にする。1RMは宣言種目だけ', () => {
    expect(settingsLine(settings, names, '上下2分割')).toBe(
      '前提: 2026-08-03 から12週・週4回（月水金日）・1回 4種目×3セット・分割 上下2分割・重点 ベンチプレス。' +
        '模擬ユーザー: 伸び 0.5%/週・初回は実力の70%・体重75kg。' +
        '初日の1RM: ベンチプレス 100kg, スクワット 140kg',
    );
  });

  // 曜日は開始日からの日数で返ってくる。開始日が月曜でなくても、実際の曜日名で書く。
  it('曜日は開始日から数えた実際の曜日名で書く', () => {
    const got = settingsLine({ ...settings, start: '2026-09-09', weekdays: [0, 2] }, names, '');
    expect(got).toContain('週4回（水金）');
  });

  // 自分の種目を足した結果は、足さない結果と数字が違う。前提の1行に出さないと、
  // どちらの条件の数字かを要約だけで読み分けられない。
  it('自分の種目があれば、名前と効く部位を書く', () => {
    const got = settingsLine(
      {
        ...settings,
        custom: [
          {
            id: 'u-sim01',
            name: 'アイソラテラル・ロー',
            // 挿入順をわざと逆にする。並びが挿入順のままなら BICEPS が
            // LAT より先に出て、regionOrder による決着を検査できない。
            stimulus: { BICEPS: 0.5, LAT: 0.5, TRAP_MID: 1 },
            increment_kg: 2.5,
          },
        ],
      },
      names,
      '',
    );
    expect(got).toContain('自分の種目: アイソラテラル・ロー（僧帽筋中部 1.0・広背筋 0.5・上腕二頭筋 0.5）');
    expect(settingsLine({ ...settings, custom: [] }, names, '')).not.toContain('自分の種目');
  });

  it('分割と重点が無ければ「なし」', () => {
    const got = settingsLine({ ...settings, focus: '', split: '' }, names, '');
    expect(got).toContain('分割 なし・重点 なし');
  });
});
