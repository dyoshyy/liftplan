import { describe, expect, it } from 'vitest';
import { lockedDeclared, toggleDeclared } from './declared';

describe('toggleDeclared', () => {
  it('入っていなければ足す', () => {
    expect(toggleDeclared(['bench'], 'squat')).toEqual(['bench', 'squat']);
  });

  it('入っていれば外す', () => {
    expect(toggleDeclared(['bench', 'squat'], 'bench')).toEqual(['squat']);
  });

  // 並びが違うと、保存のたびに画面の順序が入れ替わる。
  it('昇順に保つ', () => {
    expect(toggleDeclared(['squat'], 'bench')).toEqual(['bench', 'squat']);
  });

  it('元の配列を書き換えない', () => {
    const current = ['bench'];
    toggleDeclared(current, 'squat');
    expect(current).toEqual(['bench']);
  });
});

describe('lockedDeclared', () => {
  // 外すとサーバーが 400 を返す。画面側でそこへ到達させない。
  it('重点種目は外せない', () => {
    expect(lockedDeclared(['bench', 'squat'], 'bench').has('bench')).toBe(true);
    expect(lockedDeclared(['bench', 'squat'], 'bench').has('squat')).toBe(false);
  });

  // 宣言が空のプログラムは存在しない。
  it('最後の1つは外せない', () => {
    expect(lockedDeclared(['bench'], null).has('bench')).toBe(true);
  });

  it('2つ以上あって重点も無ければ何も固定しない', () => {
    expect(lockedDeclared(['bench', 'squat'], null).size).toBe(0);
  });

  it('重点種目が宣言に無ければ固定しない', () => {
    expect(lockedDeclared(['bench', 'squat'], 'deadlift').size).toBe(0);
  });
});
