import { describe, expect, it } from 'vitest';
import { parseBodyWeight } from './useBodyWeight';

describe('parseBodyWeight', () => {
  it('読めれば kg を返す', () => {
    expect(parseBodyWeight('75.4')).toEqual({ kg: 75.4 });
  });

  // 0 や負の体重は、打ち間違い以外にありえない。通すと自重種目の
  // 処方が壊れる（体重を足して推定し、引いて処方するため）。
  it.each(['', '  ', 'abc', '0', '-3'])('%s は断る', (raw) => {
    expect(parseBodyWeight(raw)).toEqual({ error: '体重を入れてください' });
  });
});
