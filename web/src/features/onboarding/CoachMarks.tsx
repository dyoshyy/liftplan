import { useCallback, useEffect, useLayoutEffect, useRef, useState } from 'react';
import { Button } from '../../ui/Button';
import { COACH_STEPS, nextCoachIndex, type CoachTarget } from './coach';

const find = (t: CoachTarget) => document.querySelector<HTMLElement>(`[data-coach="${t}"]`);
const present = (t: CoachTarget) => find(t) !== null;

// 切り抜きの余白。要素の縁に枠が重なると、何を指しているか読みにくい。
const PAD = 6;
// 吹き出しを下に出すのに要る高さ。足りなければ上に出す。
const BUBBLE_ROOM = 200;

// 「今日」の画面を暗くし、1か所だけ切り抜いて吹き出しで説明する。
//
// 状態はメモリだけに持つ。再読み込みで消え、二度と出ない。「済んだ」は
// 設定の保存で既に記録してあり、案内のために保存を足さない。
//
// onDone は描画をまたいで同じ関数を渡すこと。変わるたびに今の段階を
// 測り直し、スクロールし直す。
export function CoachMarks({ onDone }: { onDone: () => void }) {
  // null は「まだ測っていない」。最初の段階は描画中に決めない。描画中の
  // querySelector は前の DOM を見るので、同じ描画で「今日」のカードが
  // 入るとき全部「無い」と判断して、黙って終わる。
  const [index, setIndex] = useState<number | null>(null);
  const [rect, setRect] = useState<DOMRect | null>(null);
  const dialog = useRef<HTMLDivElement>(null);

  const advance = useCallback(
    (from: number) => {
      const n = nextCoachIndex(COACH_STEPS, from, present);
      if (n === null) onDone();
      else setIndex(n);
    },
    [onDone],
  );

  // 最初の1回だけ。以後は「次へ」が進める。
  useLayoutEffect(() => advance(0), [advance]);

  useLayoutEffect(() => {
    if (index === null) return;
    const step = COACH_STEPS[index];
    const el = step ? find(step.target) : null;
    if (!el) {
      // 測る前に消えた（記録で並びが変わった、など）。次へ送る。
      advance(index + 1);
      return;
    }
    el.scrollIntoView({ block: 'center' });
    const measure = () => setRect(el.getBoundingClientRect());
    measure();
    window.addEventListener('resize', measure);
    window.addEventListener('scroll', measure, true);
    return () => {
      window.removeEventListener('resize', measure);
      window.removeEventListener('scroll', measure, true);
    };
  }, [index, advance]);

  // 吹き出しが出たら「次へ」にフォーカスを移す。読み上げが説明から始まり、
  // そのまま Enter で進める。
  const shown = rect !== null;
  useEffect(() => {
    if (shown) dialog.current?.querySelector<HTMLButtonElement>('button:last-of-type')?.focus();
  }, [index, shown]);

  // Escape はスキップと同じ。閉じる手段がボタンだけだと、キーボードで
  // 吹き出しを見失ったときに抜けられない。
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') onDone();
    };
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  }, [onDone]);

  const step = index === null ? undefined : COACH_STEPS[index];
  if (index === null || !step || !rect) return null;

  const last = nextCoachIndex(COACH_STEPS, index + 1, present) === null;
  const below = window.innerHeight - rect.bottom > BUBBLE_ROOM;

  return (
    <>
      {/* 暗い部分を押しても進まない。誤って押して読み飛ばさないように。 */}
      <div className="fixed inset-0 z-[60]" aria-hidden="true" />
      <div
        aria-hidden="true"
        className="pointer-events-none fixed z-[61] rounded-xl ring-2 ring-amber"
        style={{
          top: rect.top - PAD,
          left: rect.left - PAD,
          width: rect.width + PAD * 2,
          height: rect.height + PAD * 2,
          boxShadow: '0 0 0 9999px rgb(0 0 0 / 0.65)',
        }}
      />
      <div
        ref={dialog}
        role="dialog"
        aria-modal="true"
        aria-label="使い方"
        className="fixed inset-x-4 z-[62] mx-auto grid max-w-[420px] gap-3 rounded-[14px] border border-line bg-surface p-4 shadow-lg"
        style={below ? { top: rect.bottom + PAD + 12 } : { bottom: window.innerHeight - rect.top + PAD + 12 }}
      >
        <p className="text-sm leading-[1.7]">{step.text}</p>
        <div className="flex items-center justify-between gap-2">
          <Button variant="quiet" size="chip" onClick={onDone}>
            スキップ
          </Button>
          <Button size="md" onClick={() => (last ? onDone() : advance(index + 1))}>
            {last ? 'はじめる' : '次へ'}
          </Button>
        </div>
      </div>
    </>
  );
}
