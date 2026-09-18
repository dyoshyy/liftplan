import { describe, expect, it } from 'vitest';
import { focusBody, focusOptions, NO_FOCUS } from './focus';

describe('focusBody', () => {
  it('種目を選んだらそのIDを送る', () => {
    expect(focusBody('bench')).toEqual({ focus_exercise: 'bench' });
  });

  // 空文字で送るとサーバーが 400 を返す。解除だけが黙って失敗する。
  it('指定なしは null で送る（空文字ではない）', () => {
    expect(focusBody(NO_FOCUS)).toEqual({ focus_exercise: null });
  });
});

describe('focusOptions', () => {
  it('先頭は指定なし', () => {
    expect(focusOptions(['bench', 'squat'])).toEqual([NO_FOCUS, 'bench', 'squat']);
  });

  // 選択種目を並べると、サーバーが弾く選択肢が画面に出る。
  it('宣言種目しか並べない', () => {
    const declared = ['bench'];
    expect(focusOptions(declared)).not.toContain('leg_press');
  });

  it('宣言が空でも指定なしは出す', () => {
    expect(focusOptions([])).toEqual([NO_FOCUS]);
  });
});
