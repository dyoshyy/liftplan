import { describe, expect, it } from 'vitest';
import { regionLabel } from './regions';

describe('regionLabel', () => {
  it('知っている区分は日本語にする', () => {
    expect(regionLabel('CHEST_MID')).toBe('大胸筋中部');
  });

  // サーバーが区分を足したときに画面が落ちるより、英語のまま出るほうがよい。
  it('知らない区分はそのまま返す', () => {
    expect(regionLabel('NEW_REGION')).toBe('NEW_REGION');
  });
});
