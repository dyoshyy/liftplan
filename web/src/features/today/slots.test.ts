import { describe, expect, it } from 'vitest';
import { slotCount } from './slots';

const planned = { sets: 3 };

describe('slotCount', () => {
  it('予定のセット数だけ枠を出す', () => {
    expect(slotCount(planned, 0)).toBe(3);
  });

  // 予定より多く記録したとき、枠が予定の数のままだと、はみ出した
  // セットが画面から消えて取り消せなくなる。
  it('予定より多く記録していたら、記録の数まで出す', () => {
    expect(slotCount(planned, 5)).toBe(5);
  });

  // 自分で選んだ種目は、終わりを決めない。何セットやるかは本人が決める
  // ので、記録のたびに次の空き枠が1つ出る。
  it('自分で選んだ種目は、記録の数より1つ多く出す', () => {
    expect(slotCount({ sets: 0, adhoc: true }, 0)).toBe(1);
    expect(slotCount({ sets: 0, adhoc: true }, 3)).toBe(4);
  });
});
