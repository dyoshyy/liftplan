import { describe, expect, it } from 'vitest';
import type { Day, RecordedSet } from '../../api/types';
import { formatKg, judgePersonalRecord, previousSets } from './pr';

const today = '2026-09-21';
const set = (id: string, weight_kg: number, reps: number, rir = 0, effective_kg?: number): RecordedSet => ({
  id,
  weight_kg,
  reps,
  rir,
  ...(effective_kg === undefined ? {} : { effective_kg }),
});

const day = (date: string, sets: RecordedSet[]): Day => ({
  date,
  exercises: [{ exercise_id: 'bench', name: 'ベンチプレス', sets }],
  total_sets: sets.length,
});

describe('previousSets', () => {
  // 今日の画面が記録のたびに見ているのは doneToday。days の今日のぶんを
  // 見ると、両者がずれたときに今日 1セット目で出した自己ベストが
  // 2セット目の判定で見えず、同じ重量でもう一度祝うことになる。
  it('今日のぶんは days ではなく doneToday から取る', () => {
    const days = [day(today, [set('stale', 200, 5)]), day('2026-09-14', [set('old', 100, 5)])];
    const doneToday = new Map([['bench', [set('fresh', 110, 5)]]]);

    const got = previousSets({ days, doneToday, exerciseId: 'bench', today });

    expect(got.map((s) => s.id).sort()).toEqual(['fresh', 'old']);
  });

  it('他の種目は混ぜない', () => {
    const days: Day[] = [
      {
        date: '2026-09-14',
        exercises: [
          { exercise_id: 'squat', name: 'スクワット', sets: [set('sq', 150, 5)] },
          { exercise_id: 'bench', name: 'ベンチプレス', sets: [set('bp', 100, 5)] },
        ],
        total_sets: 2,
      },
    ];

    const got = previousSets({ days, doneToday: new Map(), exerciseId: 'bench', today });

    expect(got.map((s) => s.id)).toEqual(['bp']);
  });

  // 直しているセット自身が母集団に残ると、自分自身を超えられない。
  // 100kg×5 を 105kg×5 に直しても「更新していない」になる。
  it('直しているセット自身は外す', () => {
    const doneToday = new Map([['bench', [set('a', 100, 5), set('b', 90, 5)]]]);

    const got = previousSets({ days: [], doneToday, exerciseId: 'bench', today, excludeId: 'a' });

    expect(got.map((s) => s.id)).toEqual(['b']);
  });
});

describe('judgePersonalRecord', () => {
  const previous = [set('a', 100, 5, 0)]; // 推定1RM 116.667

  it('過去最高を超えたら更新', () => {
    const got = judgePersonalRecord({
      exerciseId: 'bench',
      name: 'ベンチプレス',
      loadOffsetKg: 0,
      values: { weight: 105, reps: 5, rir: 0 },
      previous,
    });

    expect(got).not.toBeNull();
    expect(got?.estimatedKg).toBeCloseTo(122.5, 3);
    expect(got?.previousKg).toBeCloseTo(116.666667, 3);
    expect(got?.exerciseId).toBe('bench');
  });

  // 同値で祝うと、同じ重量・同じレップを3セット組むたびに3回出る。
  it('同値では祝わない', () => {
    const got = judgePersonalRecord({
      exerciseId: 'bench',
      name: 'ベンチプレス',
      loadOffsetKg: 0,
      values: { weight: 100, reps: 5, rir: 0 },
      previous,
    });

    expect(got).toBeNull();
  });

  it('下回れば祝わない', () => {
    const got = judgePersonalRecord({
      exerciseId: 'bench',
      name: 'ベンチプレス',
      loadOffsetKg: 0,
      values: { weight: 95, reps: 5, rir: 0 },
      previous,
    });

    expect(got).toBeNull();
  });

  // 使い始めの1セット目で全種目が発火すると、演出が「更新した合図」では
  // なく「記録したときに出るもの」になる。
  it('比べる相手が無ければ祝わない', () => {
    const got = judgePersonalRecord({
      exerciseId: 'bench',
      name: 'ベンチプレス',
      loadOffsetKg: 0,
      values: { weight: 100, reps: 5, rir: 0 },
      previous: [],
    });

    expect(got).toBeNull();
  });

  // Epley 式の適用範囲（限界までの総レップ 20）を超えると 1RM を
  // 大きく過大評価する。上限を外すと 100kg×30レップが 200kg の
  // 自己ベストとして通り、以後どんな記録でも祝われなくなる。
  it('限界までのレップが20を超えたら推定しない', () => {
    const got = judgePersonalRecord({
      exerciseId: 'bench',
      name: 'ベンチプレス',
      loadOffsetKg: 0,
      values: { weight: 100, reps: 30, rir: 0 },
      previous,
    });

    expect(got).toBeNull();
  });

  // 過去のセットに高レップが混ざっていても、そこは推定の対象外として
  // 落とす。落とさないと過大評価された値が基準になり、以後どんな記録でも
  // 祝われなくなる。
  it('過去の高レップは基準に含めない', () => {
    const got = judgePersonalRecord({
      exerciseId: 'bench',
      name: 'ベンチプレス',
      loadOffsetKg: 0,
      values: { weight: 105, reps: 5, rir: 0 },
      previous: [set('junk', 20, 100), set('a', 100, 5)],
    });

    expect(got?.previousKg).toBeCloseTo(116.666667, 3);
  });

  // 自重種目は 0kg で記録される。0 を推定1RM として母集団に残すと、
  // 初めて重りを付けた1セット目が「0kg からの更新」として祝われる。
  // 比べる相手が無いのだから、そこは祝わない。
  it('自重種目（0kg）は比べる相手として数えない', () => {
    const got = judgePersonalRecord({
      exerciseId: 'chinup',
      name: '懸垂',
      loadOffsetKg: 0,
      values: { weight: 5, reps: 5, rir: 0 },
      previous: [set('a', 0, 10)],
    });

    expect(got).toBeNull();
  });

  // RIR は「あと何回できたか」。同じ重量・レップでも、余力が残っている
  // ほうが強い。ここを見落とすと、追い込みを緩めた日の更新が出ない。
  it('RIR が大きいほど強い記録として扱う', () => {
    const got = judgePersonalRecord({
      exerciseId: 'bench',
      name: 'ベンチプレス',
      loadOffsetKg: 0,
      values: { weight: 100, reps: 5, rir: 2 },
      previous,
    });

    expect(got).not.toBeNull();
  });

  // 0.000001 の差で祝うと、量子化の残差だけで演出が出る。
  it('浮動小数点の残差では祝わない', () => {
    const got = judgePersonalRecord({
      exerciseId: 'bench',
      name: 'ベンチプレス',
      loadOffsetKg: 0,
      values: { weight: 0.1 + 0.2, reps: 1, rir: 0 },
      previous: [set('a', 0.3, 1)],
    });

    expect(got).toBeNull();
  });
});

// 自重種目は、記録した加重だけで比べると食い違う。体重を引く規則は画面に
// 持たせず、サーバーが渡す2つの数字だけで比べる。
//   - 過去のセット：その日の体重で読み替えた effective_kg
//   - いま上げるセット：記録する加重に足す量 loadOffsetKg（体重 × 自重係数）
describe('judgePersonalRecord（自重種目）', () => {
  const chin = { exerciseId: 'pull_up', name: 'チンニング' };

  // 自重10回のあとに 5kg を付けて5回やっても、強くなってはいない。
  // 加重だけで比べると前回は 0kg で「比べる相手なし」になり、5kg×5 が
  // そのまま更新として祝われていた。
  it('前回の自重10回より軽い加重5回は、祝わない', () => {
    const got = judgePersonalRecord({
      ...chin,
      values: { weight: 5, reps: 5, rir: 0 },
      loadOffsetKg: 66.5, // 0.95 × 70
      previous: [set('a', 0, 10, 0, 66.5)],
    });

    expect(got).toBeNull();
  });

  // 0kg のままでも、回数が増えれば強くなっている。自重種目を自重で続ける
  // 人の更新が、一度も祝われない。
  it('自重のままでも、回数が増えたら祝う', () => {
    const got = judgePersonalRecord({
      ...chin,
      values: { weight: 0, reps: 10, rir: 0 },
      loadOffsetKg: 66.5,
      previous: [set('a', 0, 8, 0, 66.5)],
    });

    expect(got).not.toBeNull();
    // 66.5 × (1 + 10/30)
    expect(got?.estimatedKg).toBeCloseTo(88.666667, 3);
    // 66.5 × (1 + 8/30)
    expect(got?.previousKg).toBeCloseTo(84.233333, 3);
  });

  // 過去のセットは、その日の体重で読み替えた値で比べる。いまの体重で
  // 読み替えると、減量した人の昔の記録が軽く見えて、毎回「更新」になる。
  it('過去のセットは、その日の体重込みの値で比べる', () => {
    const got = judgePersonalRecord({
      ...chin,
      // いまは体重70kg。いまの体重で読み替えた前回（84.2）なら超えているが、
      // 当時の体重80kgで読み替えた前回（96.3）は超えていない。
      values: { weight: 0, reps: 9, rir: 0 },
      loadOffsetKg: 66.5,
      previous: [set('a', 0, 8, 0, 76)], // 当時は体重80kg
    });

    expect(got).toBeNull();
  });

  // 0 は「その日の体重が引けず推定できない」。0kg として基準に残すと、
  // どんな記録でも更新になる。
  it('体重が引けず推定できない過去のセットは基準に含めない', () => {
    const got = judgePersonalRecord({
      ...chin,
      // 0 を握りつぶして「いま足す量」で読み替えると前回が 84.2 になり、
      // 今回の 88.7 が更新になってしまう。
      values: { weight: 0, reps: 10, rir: 0 },
      loadOffsetKg: 66.5,
      previous: [set('a', 0, 8, 0, 0)],
    });

    expect(got).toBeNull(); // 比べる相手が無い
  });

  // 今日この画面で記録したセットはサーバーの読み替えを持たない。同じ日なので、
  // いま足す量で読み替える。
  it('今日手元で記録したセットは、いま足す量で読み替える', () => {
    const got = judgePersonalRecord({
      ...chin,
      values: { weight: 0, reps: 10, rir: 0 },
      loadOffsetKg: 66.5,
      previous: [set('a', 0, 8)], // effective_kg が無い
    });

    expect(got?.previousKg).toBeCloseTo(84.233333, 3);
  });
});

describe('formatKg', () => {
  it('小数の残差を落として1桁までにする', () => {
    expect(formatKg(116.66666666666667)).toBe('116.7');
    expect(formatKg(125)).toBe('125');
    expect(formatKg(122.5)).toBe('122.5');
  });
});
