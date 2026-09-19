import { useSyncExternalStore } from 'react';
import { applyUpdate, hasUpdate, subscribeToUpdate } from './swUpdate';

// UpdateBanner は新しい版が待っていることを知らせる。
//
// 押すまで切り替えない。記録の途中で画面が入れ替わるほうが害が大きい。
// ただし押せる場所が無いと、古い版のまま取り残される端末ができる。
export function UpdateBanner() {
  const needRefresh = useSyncExternalStore(subscribeToUpdate, hasUpdate, () => false);

  if (!needRefresh) return null;

  return (
    <div className="card border-amber/40 bg-amber/15">
      <p className="card-title">新しい版があります</p>
      <p className="mb-3.5 text-[13px] text-muted">
        記録の途中なら、終わってから押してください。押すまで今の画面のままです。
      </p>
      <button type="button" className="btn" onClick={() => void applyUpdate()}>
        新しい版にする
      </button>
    </div>
  );
}
