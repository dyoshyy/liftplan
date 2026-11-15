import type { Enqueue } from '../../outbox/db';
import { today } from '../../domain/date';
import { Button } from '../../ui/Button';
import { formatRun } from './run';
import { RunSheet } from './RunSheet';
import { useRunRecord } from './useRunRecord';

type Props = {
  enqueue: Enqueue;
  onSaveFailed: (message: string) => void;
};

// RunRow は今日のランを1行ずつと、記録の入口を置く。
//
// 種目の予定より目立たせない。ランは計画に入らない記録で、「今日どこまで
// やるか」の外にある。置き場所は「種目を選んで記録」と同じ控えめな段。
export function RunRow({ enqueue, onSaveFailed }: Props) {
  const run = useRunRecord({ enqueue, onSaveFailed, date: today() });

  return (
    <div className="grid gap-1.5">
      {run.runs.map((r) => (
        <div key={r.id} className="flex min-h-11 items-center gap-2 px-1">
          <span className="text-[13px] text-muted">ラン</span>
          <span className="num text-[15px]">{formatRun(r)}</span>
          <Button variant="ghost" size="chip" className="ml-auto" onClick={() => void run.undo(r.id)}>
            取り消す
          </Button>
        </div>
      ))}
      <Button variant="quiet" onClick={run.open}>
        ランを記録
      </Button>
      {run.sheetOpen && <RunSheet onRecord={(v) => void run.record(v)} onClose={run.close} />}
    </div>
  );
}
