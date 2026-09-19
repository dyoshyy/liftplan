import { describe, expect, it } from 'vitest';
import { parseBodyWeight } from './useBodyWeight';

describe('parseBodyWeight', () => {
  it('読めれば kg を返す', () => {
    expect(parseBodyWeight('75.4')).toBe(75.4);
  });

  // 0 や負の体重は打ち間違い以外にありえない。通すと自重種目の処方が
  // 壊れる（体重を足して推定し、引いて処方するため）。
  //
  // 読めない値では記録ボタンを押せなくする。文で叱らない。
  it.each(['', '  ', 'abc', '0', '-3'])('%s は読めない', (raw) => {
    expect(parseBodyWeight(raw)).toBeNull();
  });

  // 入力中の「75.」は数として読める。ここで弾くと、小数点を打った瞬間に
  // ボタンが消えて戻ってくる。
  it('入力途中の 75. は読める', () => {
    expect(parseBodyWeight('75.')).toBe(75);
  });
});
