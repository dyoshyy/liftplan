import { cn } from '../ui/cn';
import type { Route } from './route';
import { syncState } from './sync';

type Props = {
  route: Route;
  onGo: (r: Route) => void;
  pending: number;
  rejected: number;
  online: boolean;
  onSync: () => void;
};

// 画面下に常時1本だけ置く。
//
// 以前は状態バー（同期済み / 更新）を常設していたが、そこに出ていたのは
// ほとんどの時間「同期済み」で、場所に見合う情報量が無かった。点1つに畳み、
// **異常なときだけ主張させる**。押すと取り直すので、「更新」ボタンも兼ねる。
export function BottomNav({ route, onGo, pending, rejected, online, onSync }: Props) {
  const { tone, label } = syncState(pending, rejected, online);

  return (
    <nav
      className="fixed inset-x-0 bottom-0 z-50 border-t border-line bg-surface/95 backdrop-blur-[10px]"
      style={{ paddingBottom: 'env(safe-area-inset-bottom)' }}
    >
      <div className="mx-auto flex max-w-[620px] items-stretch px-2">
        <button
          type="button"
          onClick={onSync}
          aria-label={label}
          title={label}
          className="flex min-w-11 items-center justify-center gap-2 px-3"
        >
          <span
            className={cn(
              'size-2.5 flex-none rounded-full',
              tone === 'trouble' ? 'bg-red' : tone === 'pending' ? 'bg-amber' : 'bg-green',
            )}
          />
          {pending > 0 && <span className="num text-[11px] text-muted">{pending}</span>}
        </button>

        <Tab label="今日" active={route === 'today'} onClick={() => onGo('today')} />
        <Tab label="履歴" active={route === 'history'} onClick={() => onGo('history')} />
      </div>
    </nav>
  );
}

function Tab({ label, active, onClick }: { label: string; active: boolean; onClick: () => void }) {
  return (
    <button
      type="button"
      onClick={onClick}
      aria-current={active ? 'page' : undefined}
      className={cn(
        'flex-1 py-3 text-center text-sm',
        // 選択中は文字を起こすだけにする。背景を敷くと、下端に色の帯が
        // 2本（ナビと選択タブ）できて、画面の重心が下がる。
        active ? 'font-bold text-text' : 'font-medium text-muted',
      )}
    >
      {label}
    </button>
  );
}
