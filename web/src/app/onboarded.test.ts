import { describe, expect, it } from 'vitest';
import { onboardedFrom } from './onboarded';

describe('onboardedFrom', () => {
  // 案内のためにアプリを止めない。次に取れたときに出る。
  it('取れなかったら済んだものとして扱う', () => {
    expect(onboardedFrom(null)).toBe(true);
  });

  it('取れたらサーバーの値を使う', () => {
    expect(onboardedFrom({ completed: false })).toBe(false);
    expect(onboardedFrom({ completed: true })).toBe(true);
  });
});
