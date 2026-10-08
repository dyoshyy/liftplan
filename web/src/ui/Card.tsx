import type { ComponentProps, HTMLAttributes, ReactNode } from 'react';
import { cn } from './cn';

type CardProps = ComponentProps<'div'> & {
  /** title は小さい見出し。大文字・字間広め・くすんだ色で、中身と competing しない。 */
  title?: ReactNode;
  /** tone は注意を引きたいカード（デロードの提案・更新の通知）に使う。 */
  tone?: 'default' | 'amber';
};

// 描かれたときに下から入る。並んだカードは index.css で上から順に遅れて出る。
// 入り方を変えたいときは className に別の animate-* を渡す（cn が後を勝たせる）。
export function Card({ title, tone = 'default', className, children, ...props }: CardProps) {
  return (
    <div
      className={cn(
        'animate-rise-in rounded-[14px] border p-4',
        tone === 'amber' ? 'border-amber/40 bg-amber/15' : 'border-line bg-surface',
        className,
      )}
      {...props}
    >
      {title !== undefined && <p className="mb-3 text-xs uppercase tracking-[0.12em] text-faint">{title}</p>}
      {children}
    </div>
  );
}

/** Note はカードの中の補足。なぜそうなっているかの説明に使う。 */
export function Note({ className, ...props }: HTMLAttributes<HTMLParagraphElement>) {
  return <p className={cn('text-xs leading-[1.7] text-faint', className)} {...props} />;
}

/** SectionTitle はカードの外に出す見出し。レーン（軸・バリエーション・補助）の
 *  区切りに使う。カードの中の title と同じ見た目にして、階層を1つに保つ。 */
export function SectionTitle({ children }: { children: ReactNode }) {
  return <p className="mb-0 text-xs uppercase tracking-[0.12em] text-faint">{children}</p>;
}
