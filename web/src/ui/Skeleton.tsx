import type { ReactNode } from 'react';
import { Card } from './Card';
import { cn } from './cn';

// Skeleton は読み込み中に、これから出るものの形だけを置く。
//
// 「読み込んでいます」の一行だと、読み終えた瞬間に画面の高さが変わって
// 中身が跳ねる。形を先に置いておけば、入れ替わっても位置がずれない。
// 寸法は本物のカードに合わせる（呼び出し側が className で渡す）。
//
// 動きを減らす設定では、index.css の全体指定で脈打ちが止まる。
export function Skeleton({ className }: { className?: string }) {
  return <div aria-hidden="true" className={cn('rounded-md bg-surface-2 animate-pulse', className)} />;
}

/** Loading は読み込み中のまとまり。読み上げには形ではなく一言を渡す。 */
export function Loading({ children, className }: { children: ReactNode; className?: string }) {
  return (
    <div role="status" aria-busy="true" className={cn('grid gap-3.5', className)}>
      <span className="sr-only">読み込んでいます</span>
      {children}
    </div>
  );
}

/** SkeletonCard は見出しと数行のカード。一覧の1件ぶん。 */
export function SkeletonCard({ rows = 2 }: { rows?: number }) {
  return (
    <Card className="grid gap-3">
      <Skeleton className="h-4 w-24" />
      {Array.from({ length: rows }, (_, i) => (
        <div key={i} className="flex items-center justify-between gap-3">
          <Skeleton className="h-4 w-32" />
          <Skeleton className="h-4 w-20" />
        </div>
      ))}
    </Card>
  );
}
