import { useId, useState, type ReactNode } from 'react';
import { cn } from './cn';

type Props = {
  title: string;
  /** summary は畳んでいるときに出す現在値。開かずに済むなら開かせない。 */
  summary?: ReactNode;
  /** defaultOpen は最初から開いておくもの。1つだけにする。 */
  defaultOpen?: boolean;
  children: ReactNode;
};

// 折りたためる節。
//
// 設定画面は縦 5464px（8.2画面分）あり、目的の項目まで**スクロールして
// 探す**状態だった。全部を同時に見る必要はないので、既定は畳んでおく。
//
// 畳んでいるときも現在値は出す。「週に通う回数」を確かめるためだけに
// 開かせるのは、畳んだ意味が無い。
export function Section({ title, summary, defaultOpen = false, children }: Props) {
  const [open, setOpen] = useState(defaultOpen);
  const id = useId();

  return (
    <div className="rounded-[14px] border border-line bg-surface">
      <button
        type="button"
        aria-expanded={open}
        aria-controls={id}
        onClick={() => setOpen((v) => !v)}
        className="flex min-h-14 w-full items-center gap-3 px-4 py-3 text-left"
      >
        <span className="font-bold">{title}</span>
        {summary !== undefined && !open && (
          <span className="ml-auto truncate text-[13px] text-muted">{summary}</span>
        )}
        <span
          className={cn(
            'text-muted transition-transform',
            summary !== undefined && !open ? 'ml-1.5' : 'ml-auto',
            open && 'rotate-180',
          )}
          aria-hidden="true"
        >
          ⌄
        </span>
      </button>

      {open && (
        <div id={id} className="border-t border-line-soft p-4">
          {children}
        </div>
      )}
    </div>
  );
}
