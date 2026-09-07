import { useRegisterSW } from 'virtual:pwa-register/react';

// UpdatePrompt は新しい版が来たことを知らせる。
//
// 自動で差し替えない（sw.ts で skipWaiting を呼ばない）。記録シートを
// 開いている最中に画面が入れ替わると、入力中の値が消える。
// ジムではそれは記録を失うのと同じ。
export function UpdatePrompt() {
  const {
    needRefresh: [needRefresh],
    updateServiceWorker,
  } = useRegisterSW();

  if (!needRefresh) return null;

  return (
    <div className="card mx-auto mb-3.5 max-w-[620px] border-amber/40 bg-amber/15">
      <p className="card-title">更新があります</p>
      <p className="mb-3.5 text-[13px] text-muted">
        記録の途中なら、終わってから押してください。押すまで今の画面のままです。
      </p>
      <button type="button" className="btn" onClick={() => void updateServiceWorker(true)}>
        新しい版にする
      </button>
    </div>
  );
}
