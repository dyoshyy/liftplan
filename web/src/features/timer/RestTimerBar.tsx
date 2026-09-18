import { Button } from '../../ui/Button';
import { cn } from '../../ui/cn';
import { formatRemaining } from './rest';
import type { RestTimer } from './useRestTimer';

// 休憩中だけ出す。
//
// 待機中は消える。自動で始まるのが通常の経路なので、待機中のバーは
// 「0:00 開始」としか言っておらず、下端の場所に見合わない。
// 長さの設定は設定画面へ移した。
//
// ナビの上に重ねる。ナビは常時1本なので、休憩中だけ2本になる。
export function RestTimerBar({ timer }: { timer: RestTimer }) {
  const { state, finished } = timer;
  if (state.kind === 'idle') return null;

  const running = state.kind === 'running';

  return (
    <div
      className={cn(
        'fixed inset-x-0 z-40 border-t backdrop-blur-[10px]',
        'bottom-[calc(49px+env(safe-area-inset-bottom))]',
        finished ? 'border-green/45 bg-green/20' : 'border-line-soft bg-surface/95',
      )}
    >
      <div className="mx-auto flex max-w-[620px] items-center gap-3 px-4 py-2.5">
        <span
          className={cn(
            'num text-[26px] font-semibold leading-none tabular-nums',
            finished ? 'text-green' : 'text-amber',
          )}
        >
          {formatRemaining(timer.remainingMs)}
        </span>
        <span className="text-[11px] text-faint">{finished ? '休憩おわり' : '休憩中'}</span>

        <div className="ml-auto flex gap-2">
          {running ? (
            <Button variant="quiet" size="chip" onClick={timer.pause}>
              一時停止
            </Button>
          ) : (
            <Button variant="quiet" size="chip" onClick={timer.resume}>
              再開
            </Button>
          )}
          <Button variant="ghost" size="chip" onClick={timer.reset}>
            やめる
          </Button>
        </div>
      </div>
    </div>
  );
}
