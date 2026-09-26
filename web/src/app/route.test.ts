import { describe, expect, it } from 'vitest';
import { fromState, isRoute } from './route';

describe('fromState', () => {
  it('積んだ行き先を読み戻す', () => {
    expect(fromState({ route: 'settings' })).toBe('settings');
    expect(fromState({ route: 'history' })).toBe('history');
  });

  // 他所が積んだ履歴に乗ることがある。壊れた値で画面が真っ白になるより、
  // 「今日」に戻るほうがよい。
  it.each([
    { name: '空', state: null },
    { name: '別物', state: { foo: 1 } },
    { name: '知らない行き先', state: { route: 'admin' } },
    { name: '文字列ではない', state: { route: 3 } },
  ])('$name なら今日に倒す', ({ state }) => {
    expect(fromState(state)).toBe('today');
  });

  it('forecast も読み戻す', () => {
    expect(fromState({ route: 'forecast' })).toBe('forecast');
  });
});

describe('isRoute', () => {
  it('知っている行き先だけを通す', () => {
    expect(isRoute('today')).toBe(true);
    expect(isRoute('nope')).toBe(false);
  });

  it('forecast も知っている行き先', () => {
    expect(isRoute('forecast')).toBe(true);
  });
});
