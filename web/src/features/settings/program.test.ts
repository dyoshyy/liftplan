import { describe, expect, it } from 'vitest';
import { asText, isDirty, parseTarget } from './program';

describe('parseTarget', () => {
  it('数字なら数値にする', () => {
    expect(parseTarget({ CHEST_MID: '14', LAT: '12.5' })).toEqual({
      ok: true,
      value: { CHEST_MID: 14, LAT: 12.5 },
    });
  });

  // 数字でない値をそのまま送ると、サーバーの検証に当たるか、最悪
  // 週目標が壊れる。どの区分が悪いのかまで言わないと直せない。
  it('数字でなければ、どの区分かを言って断る', () => {
    const got = parseTarget({ CHEST_MID: '14', LAT: 'ほげ' });
    expect(got.ok).toBe(false);
    if (!got.ok) expect(got.region).toBe('LAT');
  });

  it.each(['', ' ', 'abc'])('%s は断る', (bad) => {
    expect(parseTarget({ LAT: bad }).ok).toBe(false);
  });
});

describe('isDirty', () => {
  // 並び順の違いだけで「変わった」にすると、押していないのに保存ボタンが
  // 出続ける。逆に中身の違いを見落とすと、変えたのに保存できない。
  it('並び順が違うだけなら変わっていない', () => {
    expect(isDirty(['squat', 'bench'], ['bench', 'squat'])).toBe(false);
  });

  it('中身が違えば変わっている', () => {
    expect(isDirty(['bench'], ['bench', 'squat'])).toBe(true);
  });

  it('片方が空でも比べられる', () => {
    expect(isDirty([], ['bench'])).toBe(true);
    expect(isDirty([], [])).toBe(false);
  });
});

describe('asText', () => {
  // 数値のまま持つと、入力中の「1.」や空欄が NaN になって値が飛ぶ。
  it('入力欄に入れられる文字列にする', () => {
    expect(asText({ LAT: 12.5, ABS: 7 })).toEqual({ LAT: '12.5', ABS: '7' });
  });
});
