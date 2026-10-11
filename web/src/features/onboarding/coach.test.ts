import { describe, expect, it } from 'vitest';
import { COACH_STEPS, nextCoachIndex } from './coach';

const all = () => true;

describe('nextCoachIndex', () => {
  it('全部あれば最初から', () => {
    expect(nextCoachIndex(COACH_STEPS, 0, all)).toBe(0);
  });

  // 休みの日や全部済んだ日はカードが無い。無い場所を指して暗くすると、
  // 何も説明されないまま画面が操作できなくなる。
  it('指す要素が無い段階を飛ばす', () => {
    expect(nextCoachIndex(COACH_STEPS, 0, (t) => t !== 'target' && t !== 'slot')).toBe(2);
  });

  it('全部無ければ出さない', () => {
    expect(nextCoachIndex(COACH_STEPS, 0, () => false)).toBeNull();
  });

  it('最後の次で終わる', () => {
    expect(nextCoachIndex(COACH_STEPS, COACH_STEPS.length, all)).toBeNull();
  });
});

describe('COACH_STEPS', () => {
  // 重さ → 記録 → やめどき → 休憩 → 設定。記録の仕方を先に言わないと、
  // 1セット目を押す前に読むのをやめた人が記録できない。
  it('仕様の順に5段階', () => {
    expect(COACH_STEPS.map((s) => s.target)).toEqual(['target', 'slot', 'lane', 'rest', 'settings']);
  });
});
