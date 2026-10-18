import { describe, expect, it } from 'vitest';
import { isBackdropClick } from './backdrop';

// シートの <dialog> は余白が 0 で、中身の div が全面を覆う。だから押した先が
// <dialog> そのものなら、それは暗い部分（::backdrop）を押したことになる。
describe('isBackdropClick', () => {
  const dialog = { name: 'dialog' };
  const inside = { name: 'button' };

  it('暗い部分で押して離したら閉じる', () => {
    expect(isBackdropClick(dialog, dialog, dialog)).toBe(true);
  });

  it('シートの中を押しても閉じない', () => {
    expect(isBackdropClick(dialog, inside, inside)).toBe(false);
  });

  // 入力欄の文字を選んでいて指が外に出たとき。click は押した先と離した先の
  // 共通の祖先（<dialog>）に届くので、離した先だけ見ると閉じてしまう。
  it('シートの中で押して暗い部分で離しても閉じない', () => {
    expect(isBackdropClick(dialog, inside, dialog)).toBe(false);
  });

  it('暗い部分で押してシートの中で離しても閉じない', () => {
    expect(isBackdropClick(dialog, dialog, inside)).toBe(false);
  });

  it('押した先が分からなければ閉じない', () => {
    expect(isBackdropClick(dialog, null, dialog)).toBe(false);
  });
});
