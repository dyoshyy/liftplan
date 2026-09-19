import type { InputHTMLAttributes, ReactNode, SelectHTMLAttributes } from 'react';
import { useId } from 'react';
import { cn } from './cn';

// min-h-11（44px）は指で押す的の下限。p-3 だけだと font-size 次第で
// 42px になることがある。
const control =
  'min-h-11 w-full rounded-[10px] border border-line bg-surface-2 p-3 text-inherit ' +
  'focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-amber';

export function Field({ label, children }: { label?: ReactNode; children: ReactNode }) {
  return (
    <div className="grid gap-1.5">
      {label !== undefined && (
        <span className="text-xs tracking-[0.04em] text-muted">{label}</span>
      )}
      {children}
    </div>
  );
}

export function Input({ className, ...props }: InputHTMLAttributes<HTMLInputElement>) {
  return <input className={cn(control, className)} {...props} />;
}

export function Select({ className, ...props }: SelectHTMLAttributes<HTMLSelectElement>) {
  return <select className={cn(control, className)} {...props} />;
}

/** LabeledInput は label と input を id で結ぶ。読み上げと、ラベルを押して
 *  入力へ移れることのため。手で id を振ると、増えるたびに衝突を気にすることになる。 */
export function LabeledInput({
  label,
  className,
  ...props
}: InputHTMLAttributes<HTMLInputElement> & { label: ReactNode }) {
  const id = useId();
  return (
    <div className="grid gap-1.5">
      <label htmlFor={id} className="text-xs tracking-[0.04em] text-muted">
        {label}
      </label>
      <Input id={id} className={className} {...props} />
    </div>
  );
}
