import { describe, expect, it } from 'vitest';
import { fromState, isRoute, planMove, type Route } from './route';

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

  it('exercises も読み戻す', () => {
    expect(fromState({ route: 'exercises' })).toBe('exercises');
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

  it('exercises も知っている行き先', () => {
    expect(isRoute('exercises')).toBe(true);
  });
});

// 行き先の移り方。履歴の積み方を決める判断はここにだけ置き、useRoute は
// 実行するだけにする（フックの中で分岐すると DOM を立てないと検査できない）。
//
// 種目ページは設定の子（設定 → 種目 の2段）。以前は種目へも置き換えで移って
// いたので、種目ページで戻るジェスチャーをすると設定ではなく今日に戻った
// （「← 設定」を押す操作と食い違う。#228）。
describe('planMove', () => {
  it.each<{ name: string; from: Route; to: Route; want: ReturnType<typeof planMove> }>([
    { name: '同じ行き先なら何もしない', from: 'settings', to: 'settings', want: { kind: 'stay' } },
    { name: '今日から移るときは積む', from: 'today', to: 'settings', want: { kind: 'push' } },
    { name: '今日へは戻る（積まない）', from: 'history', to: 'today', want: { kind: 'back', steps: 1 } },
    { name: '並びの画面どうしは置き換える', from: 'history', to: 'settings', want: { kind: 'replace' } },
    // ここからが #228
    { name: '設定から種目へは1段積む', from: 'settings', to: 'exercises', want: { kind: 'push' } },
    { name: '種目から設定へは1段戻る', from: 'exercises', to: 'settings', want: { kind: 'back', steps: 1 } },
    // 1段だけ戻ると設定に着き、もう一度戻らないとアプリを抜けられない
    { name: '種目から今日へは2段戻る', from: 'exercises', to: 'today', want: { kind: 'back', steps: 2 } },
    // 置き換えだけだと「今日 → 設定 → 履歴」と積み残り、履歴で戻ると設定に出る
    {
      name: '種目から履歴へは1段戻ってから置き換える',
      from: 'exercises',
      to: 'history',
      want: { kind: 'back', steps: 1, then: 'history' },
    },
  ])('$name', ({ from, to, want }) => {
    expect(planMove(from, to)).toEqual(want);
  });
});
