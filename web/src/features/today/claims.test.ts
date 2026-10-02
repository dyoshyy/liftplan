import { describe, expect, it } from 'vitest';
import { createClaims } from './claims';

describe('createClaims', () => {
  // 押した瞬間に1回だけ通す。記録は、休憩を始める・自己ベストの演出を出す、
  // といった冪等でない副作用を持つ。同じ操作を2回通すと、演出が2回出る。
  it('同じキーは、最初の1回だけ通す', () => {
    const claims = createClaims();

    expect(claims.claim('sheet-1:record')).toBe(true);
    expect(claims.claim('sheet-1:record')).toBe(false);
    expect(claims.claim('sheet-1:record')).toBe(false);
  });

  // キーは「シートを開いた1回の操作」で決まる。別のシート（次のセット）の
  // 記録まで止めると、保存を待つあいだに続けて記録した分が黙って消える。
  it('別のキーは互いに止めない', () => {
    const claims = createClaims();

    expect(claims.claim('sheet-1:record')).toBe(true);
    expect(claims.claim('sheet-2:record')).toBe(true);
  });

  // 同じシートでも、記録と取り消しは別の操作。
  it('同じシートの記録と取り消しは別', () => {
    const claims = createClaims();

    expect(claims.claim('sheet-1:record')).toBe(true);
    expect(claims.claim('sheet-1:undo')).toBe(true);
  });

  // 呼び手ごとに別。1つの画面のラッチが、別の画面の操作を止めない。
  it('別のラッチは状態を共有しない', () => {
    const a = createClaims();
    const b = createClaims();

    expect(a.claim('k')).toBe(true);
    expect(b.claim('k')).toBe(true);
  });
});
