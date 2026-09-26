import { describe, expect, it } from 'vitest';
import { describePutFailure, isDirty } from './useProgramSettings';

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

// サーバーが本文に書いた理由（5分割の頻度下限のような、状態コードだけでは
// 本人に伝わらない理由）をそのまま出す。理由が無ければ状態コードだけを出す
// （web/src/dev/simulate.ts の describeFailure と同じ考え方）。
describe('describePutFailure', () => {
  it('サーバーが書いた理由をそのまま出す', () => {
    const message = '5分割は週4回以上が必要です。先に分割を変えてください';
    expect(describePutFailure(400, { error: message })).toBe(message);
  });

  it('理由が空文字なら状態コードだけを出す', () => {
    expect(describePutFailure(400, { error: '' })).toBe('変えられませんでした（400）');
  });

  it('本文が読めなければ状態コードだけを出す', () => {
    expect(describePutFailure(500, null)).toBe('変えられませんでした（500）');
  });
});
