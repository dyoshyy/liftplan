/// <reference lib="webworker" />
import { precacheAndRoute } from 'workbox-precaching';

declare const self: ServiceWorkerGlobalScope & {
  // ビルド時に vite-plugin-pwa が差し込むプリキャッシュの一覧。
  __WB_MANIFEST: Parameters<typeof precacheAndRoute>[0];
};

// 画面の殻だけをキャッシュする。
//
// API は別オリジン（Cloud Run）にあるので、ここには来ない。
// 「古いメニューを出さない」は設定ではなく構造上の事実になった。
// 同一オリジンに置いていた頃は /api を除外する分岐で守っていた。
//
// 一覧はビルド時に差し込まれる。資産の名前にハッシュが付くので、
// 手で書くと更新のたびにずれる。
precacheAndRoute(self.__WB_MANIFEST);

// skipWaiting を自動で呼ばない。
//
// 呼ぶと、記録シートを開いている最中に新しい版へ差し替わりうる。
// 入力中の値が消えるのは、ジムでは記録を失うのと同じ。
// 画面が「更新があります」を出し、利用者が押したときだけ差し替える。
self.addEventListener('message', (event) => {
  if ((event.data as { type?: string } | undefined)?.type === 'SKIP_WAITING') {
    void self.skipWaiting();
  }
});
