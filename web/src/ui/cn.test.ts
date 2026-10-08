import { describe, expect, it } from 'vitest';
import { cn } from './cn';

describe('cn', () => {
  // Card は既定で animate-rise-in を持つ。バナーが渡した入りと両方残ると、
  // どちらが効くかが CSS の並び順で決まる。
  it('あとから渡した animate-* が先のものに勝つ', () => {
    expect(cn('animate-rise-in p-4', 'animate-drop-in')).toBe('p-4 animate-drop-in');
  });

  it('animate-* と別の性質のユーティリティは両方残す', () => {
    expect(cn('animate-grow-x', 'origin-left')).toBe('animate-grow-x origin-left');
  });
});
