import { describe, expect, it } from 'vitest';
import { isDirty } from './useProgramSettings';

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
