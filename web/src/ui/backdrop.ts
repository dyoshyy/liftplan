import { useRef, type PointerEvent, type MouseEvent, type RefObject } from 'react';

/**
 * isBackdropClick は、押した先と離した先がどちらも <dialog> そのもの
 * （＝暗い部分）だったかを返す。
 *
 * 離した先だけでは決めない。シートの中で押して外で離すと、click は両方の
 * 共通の祖先である <dialog> に届く。入力欄の文字を選んでいて指がはみ出した
 * だけで閉じると、入れた値が消える。
 */
export function isBackdropClick(dialog: unknown, pressed: unknown, released: unknown): boolean {
  return pressed === dialog && released === dialog;
}

/** useBackdropClose は、暗い部分を押したら <dialog> を閉じるハンドラを返す。 */
export function useBackdropClose(ref: RefObject<HTMLDialogElement | null>) {
  const pressed = useRef<EventTarget | null>(null);
  return {
    onPointerDown: (e: PointerEvent<HTMLDialogElement>) => {
      pressed.current = e.target;
    },
    onClick: (e: MouseEvent<HTMLDialogElement>) => {
      if (isBackdropClick(ref.current, pressed.current, e.target)) ref.current?.close();
    },
  };
}
