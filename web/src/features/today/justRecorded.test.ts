import { describe, expect, it } from 'vitest';
import type { RecordedSet } from '../../api/types';
import { justRecorded } from './justRecorded';

const set = (id: string): RecordedSet => ({ id, weight_kg: 100, reps: 5, rir: 2 });

describe('justRecorded', () => {
  const seen = new Set(['a', 'b']);

  it.each([
    // 開いた時点で済んでいたセットが弾むと、開くたびに全部の枠が跳ねる。
    { name: '描いた時点で済んでいたセットは弾まない', rec: set('a'), want: false },
    { name: 'あとから記録したセットは弾む', rec: set('c'), want: true },
    { name: 'まだ記録していない枠は弾まない', rec: undefined, want: false },
  ])('$name', ({ rec, want }) => {
    expect(justRecorded(seen, rec)).toBe(want);
  });
});
