import { useEffect, useMemo, type CSSProperties } from 'react';
import { PartyParrot } from './PartyParrot';
import { formatKg, type PersonalRecord } from './pr';

// 自己ベストを更新した瞬間の演出。
//
// **ここは描画だけ。**何が更新かは `pr.ts` が決めている。この部品は
// 「更新だった」を受け取って見せるだけで、判断を持たない。
//
// 派手にしてあるのは、ジムで画面を見ている時間が一瞬だからで、
// 控えめに出すと気づかないまま次のセットに入る。逆に**操作は止めない**。
// 今日のリストは上から消していって好きなところでやめられるキューなので、
// 次の1手を数秒ブロックしない。触れば即座に消える。

/** HOLD_MS は自動で消えるまで。触ればそれより早く消える。 */
const HOLD_MS = 2600;

/** SHARDS は飛び散る破片の数。IPF のプレート色を順に割り当てる。 */
const SHARDS = 44;

const COLORS = [
  'var(--color-amber)',
  'var(--color-green)',
  'var(--color-red)',
  'var(--color-text)',
];

/** PARROTS は跳ねるパロットの置き場所。左右に振り分けて、中央の数字を隠さない。 */
const PARROTS: { left?: string; right?: string; bottom: string; size: number; delay: string }[] = [
  { left: '3%', bottom: '14%', size: 68, delay: '0s' },
  { left: '8%', bottom: '62%', size: 48, delay: '0.14s' },
  { right: '3%', bottom: '16%', size: 68, delay: '0.07s' },
  { right: '8%', bottom: '64%', size: 48, delay: '0.21s' },
  { left: '44%', bottom: '4%', size: 54, delay: '0.28s' },
];

// VIBRATION は「ドン・ドン・ドドン」。端末が対応していなければ何も起きない。
const VIBRATION = [35, 50, 35, 50, 110];

type Props = {
  pr: PersonalRecord;
  onDone: () => void;
};

// prefersReducedMotion は演出を出す時点の設定を読む。
const prefersReducedMotion = (): boolean =>
  typeof matchMedia === 'function' && matchMedia('(prefers-reduced-motion: reduce)').matches;

export function PRCelebration({ pr, onDone }: Props) {
  const reduced = useMemo(prefersReducedMotion, []);

  // 破片の散り方は1回だけ決める。描画のたびに引き直すと、再描画で
  // 弾道が飛ぶ。
  const shards = useMemo(
    () =>
      Array.from({ length: SHARDS }, (_, i) => {
        const spread = (i / SHARDS) * 360 + (Math.random() * 16 - 8);
        return {
          key: i,
          style: {
            '--a': `${spread}deg`,
            '--d': `${130 + Math.random() * 230}px`,
            '--r': `${Math.random() * 900 - 450}deg`,
            '--t': `${0.68 + Math.random() * 0.5}s`,
            '--delay': `${Math.random() * 0.14}s`,
            '--c': COLORS[i % COLORS.length],
            '--w': `${5 + Math.random() * 7}px`,
            '--h': `${11 + Math.random() * 16}px`,
          } as CSSProperties,
        };
      }),
    [],
  );

  useEffect(() => {
    // iOS Safari には無い。PWA の利用者の半分はここを通らない。
    navigator.vibrate?.(reduced ? 0 : VIBRATION);
    const t = setTimeout(onDone, HOLD_MS);
    return () => clearTimeout(t);
  }, [onDone, reduced]);

  return (
    <div
      className="pr-overlay"
      data-reduced={reduced ? '' : undefined}
      role="status"
      aria-live="polite"
      onClick={onDone}
    >
      {!reduced && (
        <>
          <div className="pr-flash" aria-hidden="true" />
          <div className="pr-rays" aria-hidden="true" />
          <div className="pr-ring" aria-hidden="true" />
          <div className="pr-ring pr-ring-2" aria-hidden="true" />
          <div className="pr-ring pr-ring-3" aria-hidden="true" />
          <div className="pr-shards" aria-hidden="true">
            {shards.map((s) => (
              <i key={s.key} className="pr-shard" style={s.style} />
            ))}
          </div>
          {PARROTS.map((p, i) => (
            <PartyParrot
              key={i}
              style={
                {
                  left: p.left,
                  right: p.right,
                  bottom: p.bottom,
                  width: p.size,
                  height: p.size,
                  '--delay': p.delay,
                } as CSSProperties
              }
            />
          ))}
        </>
      )}

      <div className="pr-card">
        <div className="pr-badge num" aria-hidden="true">
          PR
        </div>
        <div className="pr-title">自己ベスト更新</div>
        <div className="pr-name">{pr.name}</div>
        <div className="pr-kg num">
          {formatKg(pr.estimatedKg)}
          <span className="pr-unit">kg</span>
        </div>
        <div className="pr-delta num">
          +{formatKg(pr.deltaKg)}kg<span className="pr-prev">（前 {formatKg(pr.previousKg)}kg）</span>
        </div>
        {/* 推定であることは隠さない。実測の1RMだと思われると、次に
            その重量を持とうとする人が出る。 */}
        <div className="pr-note">推定1RM・直近8週の記録から</div>
      </div>
    </div>
  );
}
