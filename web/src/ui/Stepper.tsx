import { useId } from 'react';
import { Button } from './Button';
import { cn } from './cn';

type Props = {
  label: React.ReactNode;
  value: string;
  onChange: (v: string) => void;
  /** step は ＋/− で動く量。 */
  step: number;
  decimal?: boolean;
  /** min は下限。既定は 0。負の重量やレップは意味を持たない。 */
  min?: number;
  /** suffix は入力の右に出す単位。「分」など。 */
  suffix?: string;
  className?: string;
};

// 数値入力にボタンを添える。
//
// ブラウザ既定のスピナーは指で押せる大きさにならない。汗ばんだ手でも
// 押せる大きさが要る。
//
// 記録シートと休憩タイマーの両方が使う。片方だけ押しやすさが違う状態を
// 作らないために、ここ1箇所に置く。
export function Stepper({
  label,
  value,
  onChange,
  step,
  decimal,
  min = 0,
  suffix,
  className,
}: Props) {
  const id = useId();

  const bump = (by: number) => {
    const now = Number.parseFloat(value);
    const next = (Number.isFinite(now) ? now : 0) + by;
    // 0.1 の足し算で 2.7000000000000002 が出るので丸める。
    onChange(String(Math.max(min, Math.round(next * 100) / 100)));
  };

  return (
    <div className={cn('grid gap-1.5', className)}>
      <label htmlFor={id} className="text-xs tracking-[0.04em] text-muted">
        {label}
      </label>
      <div className="grid grid-cols-[60px_1fr_60px] gap-2">
        <Button variant="quiet" size="icon" className="w-full" onClick={() => bump(-step)}
          aria-label={`${step} 減らす`}>
          −
        </Button>
        <div className="relative">
          <input
            id={id}
            type="number"
            inputMode={decimal ? 'decimal' : 'numeric'}
            step={decimal ? 0.5 : 1}
            value={value}
            onChange={(e) => onChange(e.target.value)}
            className={cn(
              'num w-full rounded-[10px] border border-line bg-surface-2 p-3 text-center text-[22px] text-inherit',
              'focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-amber',
              suffix && 'pr-9',
            )}
          />
          {suffix && (
            <span className="pointer-events-none absolute right-3 top-1/2 -translate-y-1/2 text-sm text-muted">
              {suffix}
            </span>
          )}
        </div>
        <Button variant="quiet" size="icon" className="w-full" onClick={() => bump(step)}
          aria-label={`${step} 増やす`}>
          ＋
        </Button>
      </div>
    </div>
  );
}
