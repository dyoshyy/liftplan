import { describe, expect, it } from 'vitest';
import type { Session } from '../api/types';
import { awaitingFirstLoad } from './firstLoad';

const session = { date: '2026-10-09', main: [], variation: [], accessories: [] } as unknown as Session;

describe('awaitingFirstLoad', () => {
  it('まだ何も無く、読んでいる最中なら待ち', () => {
    expect(awaitingFirstLoad('loading', null)).toBe(true);
  });

  // 読み直し（設定の変更・電波の復帰・引っ張って更新）でも load は loading に
  // 戻る。手元にメニューがあるのにスケルトンへ替えると、記録の途中で画面が消える。
  it('手元にメニューがあれば、読み直し中でも待ちにしない', () => {
    expect(awaitingFirstLoad('loading', session)).toBe(false);
  });

  // 圏外で開いたときは読み込みが終わっている。スケルトンを出し続けると、
  // 読めなかったことが分からない。
  it.each(['offline', 'unauthorized', 'ready'] as const)('%s なら待ちにしない', (load) => {
    expect(awaitingFirstLoad(load, null)).toBe(false);
  });
});
