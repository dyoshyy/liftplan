import { describe, expect, it } from 'vitest';
import type { Day, RecordedSet } from '../../api/types';
import { formatKg, judgePersonalRecord, previousSets } from './pr';

const today = '2026-09-21';
const set = (id: string, weight_kg: number, reps: number, rir = 0): RecordedSet => ({
  id,
  weight_kg,
  reps,
  rir,
});

const day = (date: string, sets: RecordedSet[]): Day => ({
  date,
  exercises: [{ exercise_id: 'bench', name: 'ベンチプレス', sets }],
  total_sets: sets.length,
});

describe('previousSets', () => {
  // days の今日のぶんはサーバーから取ったままで、recordLocally では
  // 進まない。落とさずに使うと、今日 1セット目で出した自己ベストが
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
      values: { weight: 100, reps: 5, rir: 0 },
      previous,
    });

    expect(got).toBeNull();
  });

  it('下回れば祝わない', () => {
    const got = judgePersonalRecord({
      exerciseId: 'bench',
      name: 'ベンチプレス',
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
      values: { weight: 0.1 + 0.2, reps: 1, rir: 0 },
      previous: [set('a', 0.3, 1)],
    });

    expect(got).toBeNull();
  });
});

describe('formatKg', () => {
  it('小数の残差を落として1桁までにする', () => {
    expect(formatKg(116.66666666666667)).toBe('116.7');
    expect(formatKg(125)).toBe('125');
    expect(formatKg(122.5)).toBe('122.5');
  });
});
