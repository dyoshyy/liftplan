import { Card } from '../../ui/Card';
import { Loading, Skeleton } from '../../ui/Skeleton';

// TodaySkeleton は今日のメニューを初めて読むあいだの形。
// 種目カード（ExerciseCard）の並びに合わせ、読み終えたときに跳ねないようにする。
export function TodaySkeleton() {
  return (
    <Loading>
      <Skeleton className="h-11 w-40 rounded-xl" />
      <Skeleton className="h-3 w-10" />
      {[0, 1, 2].map((i) => (
        <Card key={i} className="grid gap-3">
          <Skeleton className="h-5 w-36" />
          <div className="flex items-baseline gap-2.5">
            <Skeleton className="h-8 w-24" />
            <Skeleton className="h-3.5 w-36" />
          </div>
          <div className="grid grid-cols-3 gap-2">
            {[0, 1, 2].map((s) => (
              <Skeleton key={s} className="h-16 rounded-xl" />
            ))}
          </div>
        </Card>
      ))}
    </Loading>
  );
}
