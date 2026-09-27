import { describe, expect, it } from 'vitest';
import { regionLabel, regionOrder } from './regions';

describe('regionLabel', () => {
  it('知っている区分は日本語にする', () => {
    expect(regionLabel('CHEST_MID')).toBe('大胸筋中部');
  });

  // サーバーが区分を足したときに画面が落ちるより、英語のまま出るほうがよい。
  it('知らない区分はそのまま返す', () => {
    expect(regionLabel('NEW_REGION')).toBe('NEW_REGION');
  });
});

describe('regionOrder', () => {
  // 体の上から下の並び。文字コード順だと LAT が BICEPS より後ろに来て
  // 食い違う（L > B）。
  it('体の上から下の並びで、文字コード順とは違う', () => {
    expect(regionOrder('LAT')).toBeLessThan(regionOrder('BICEPS'));
    expect('LAT' > 'BICEPS').toBe(true);
  });

  // 知らない区分は末尾。並びの外に置いても落ちない。
  it('知らない区分は既知の区分より後ろにする', () => {
    expect(regionOrder('NEW_REGION')).toBeGreaterThan(regionOrder('OBLIQUE'));
  });
});
