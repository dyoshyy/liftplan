import { registerSW } from 'virtual:pwa-register';

// Service Worker の登録と、新しいバージョンが待っていることの通知。
//
// **登録は画面の部品に置かない。**部品が描かれるまで登録されないので、
// 以前それで初回訪問の登録が漏れていた。ここはモジュールの読み込みで走る。
//
// **自動で差し替えない。**skipWaiting を呼ぶと、記録シートを開いている
// 最中に画面が入れ替わり、入力中の値が消える。
//
// ただし「勝手に差し替えない」だけでは足りなかった。待機中のバージョンは
// **全てのクライアントが閉じたとき**にしか有効にならないので、ホーム画面の
// PWA を開いたままだと古いバージョンを実行し続ける。実際、API の向き先を間違えた
// バンドルを一度配ってしまい、直した版を出しても端末側が移らなかった。
// 画面から押せる逃げ道が要る。

let needRefresh = false;
const listeners = new Set<() => void>();

const updateSW = registerSW({
  immediate: true,
  onNeedRefresh() {
    needRefresh = true;
    for (const notify of listeners) notify();
  },
});

export function subscribeToUpdate(notify: () => void): () => void {
  listeners.add(notify);
  return () => {
    listeners.delete(notify);
  };
}

export const hasUpdate = (): boolean => needRefresh;

/** applyUpdate は待機中のバージョンに切り替えて読み込み直す。押されたときだけ呼ぶ。 */
export const applyUpdate = (): Promise<void> => updateSW(true);
