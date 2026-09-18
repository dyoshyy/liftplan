import { describe, expect, it } from 'vitest';
import { syncState } from './sync';

describe('syncState', () => {
  // 捨てた記録が一番強い。オフラインは電波が戻れば直るが、捨てた記録は
  // 本人が入れ直さない限り戻らない。
  it('送れなかった記録はオフラインより優先する', () => {
    expect(syncState(0, 2, false).tone).toBe('trouble');
    expect(syncState(0, 2, false).label).toContain('送れなかった');
  });

  it.each([
    { name: '正常', pending: 0, rejected: 0, online: true, tone: 'ok' },
    { name: '未送信あり', pending: 3, rejected: 0, online: true, tone: 'pending' },
    { name: 'オフライン', pending: 0, rejected: 0, online: false, tone: 'trouble' },
  ])('$name → $tone', ({ pending, rejected, online, tone }) => {
    expect(syncState(pending, rejected, online).tone).toBe(tone);
  });

  it('押すと何が起きるかまで言う', () => {
    expect(syncState(0, 0, true).label).toContain('押すと');
    expect(syncState(2, 0, true).label).toContain('押すと');
  });

  it('未送信の件数を出す', () => {
    expect(syncState(3, 0, false).label).toBe('オフライン・未送信 3 件');
  });
});
