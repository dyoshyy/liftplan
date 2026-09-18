import { useState } from 'react';
import { Button } from '../../ui/Button';
import { Stepper } from '../../ui/Stepper';
import { cn } from '../../ui/cn';
import { formatRemaining } from './rest';
import type { RestTimer } from './useRestTimer';

const STEP_SEC = 15;

// 状態バーのすぐ上に固定する。
//
// 記録シートを閉じても、カードをスクロールしても見えている必要がある。
// セットの合間に一瞬だけ視線が落ちる前提なので、残り時間は大きく出す。
export function RestTimerBar({ timer }: { timer: RestTimer }) {
  const [editing, setEditing] = useState(false);
  const { state, finished } = timer;
  const running = state.kind === 'running';

  return (
    <div
      className={cn(
        'fixed inset-x-0 z-40 border-t backdrop-blur-[10px]',
        'bottom-[calc(53px+env(safe-area-inset-bottom))]',
        finished ? 'border-green/45 bg-green/20' : 'border-line-soft bg-surface/95',
      )}
    >
      {editing && (
        <div className="mx-auto max-w-[620px] border-b border-line-soft px-4 py-3">
          <Stepper
            label="休憩の長さ"
            value={String(Math.round(timer.durationSec / 60 * 100) / 100)}
            onChange={(v) => timer.setDurationSec(Number.parseFloat(v) * 60)}
            step={STEP_SEC / 60}
            decimal
            min={0}
            suffix="分"
          />
        </div>
      )}

      <div className="mx-auto flex max-w-[620px] items-center gap-3 px-4 py-2.5">
        <button
          type="button"
          onClick={() => setEditing((v) => !v)}
          aria-expanded={editing}
          className="text-left"
        >
          <span
            className={cn(
              'num text-[26px] font-semibold leading-none tabular-nums',
              finished ? 'text-green' : running ? 'text-amber' : 'text-muted',
            )}
          >
            {formatRemaining(timer.remainingMs)}
          </span>
          <span className="ml-2 text-[11px] text-faint">
            {finished ? '休憩おわり' : running ? '休憩中' : '長さを変える'}
          </span>
        </button>

        <div className="ml-auto flex gap-2">
          {running ? (
            <Button variant="quiet" size="chip" onClick={timer.pause}>
              一時停止
            </Button>
          ) : state.kind === 'paused' ? (
            <Button variant="quiet" size="chip" onClick={timer.resume}>
              再開
            </Button>
          ) : (
            <Button variant="quiet" size="chip" onClick={timer.start}>
              開始
            </Button>
          )}
          {state.kind !== 'idle' && (
            <Button variant="ghost" size="chip" onClick={timer.reset}>
              リセット
            </Button>
          )}
        </div>
      </div>
    </div>
  );
}
