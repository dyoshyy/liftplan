import { describe, expect, it } from 'vitest';
import { fillPercent } from './volume';

describe('fillPercent', () => {
  it('目標の半分なら 50', () => {
    expect(fillPercent(5, 10)).toBe(50);
  });

  // 棒が枠から溢れると、超えた量ではなく「壊れている」に見える。
  it('目標を超えても 100 で止める', () => {
    expect(fillPercent(30, 10)).toBe(100);
  });

  // 週目標に 0 の区分は入らない（サーバーが下限 0.5 で弾く）が、
  // 割る側に来た瞬間 Infinity になり NaN% が style に入る。
  it('目標が 0 でも実績が無ければ 0', () => {
    expect(fillPercent(0, 0)).toBe(0);
  });

  it('目標が 0 で実績があれば 100', () => {
    expect(fillPercent(3, 0)).toBe(100);
  });

  it('整数に丸める', () => {
    expect(fillPercent(1, 3)).toBe(33);
  });
});
