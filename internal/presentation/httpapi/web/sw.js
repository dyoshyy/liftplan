// 画面の殻だけをキャッシュする。
//
// API の応答はキャッシュしない。古いメニューを出すくらいなら、
// 出ないほうがよい。「昨日の重量が表示されているのに気づかない」は
// 記録そのものを壊す。
const SHELL = 'liftplan-shell-v2';
const FILES = ['/', '/app.js', '/icon.svg', '/app.webmanifest'];

self.addEventListener('install', (e) => {
  e.waitUntil(caches.open(SHELL).then((c) => c.addAll(FILES)).then(() => self.skipWaiting()));
});

self.addEventListener('activate', (e) => {
  e.waitUntil(
    caches.keys()
      .then((keys) => Promise.all(keys.filter((k) => k !== SHELL).map((k) => caches.delete(k))))
      .then(() => self.clients.claim()),
  );
});

self.addEventListener('fetch', (e) => {
  const url = new URL(e.request.url);
  if (url.pathname.startsWith('/api/') || e.request.method !== 'GET') return;

  // 自分のところのファイルだけを扱う。全部を通すと、フォントの
  // サブセットが1文字種ごとに殻へ入り、上限なく増える。
  // Google Fonts は日本語を100以上のファイルに分けて配っている。
  if (url.origin !== self.location.origin) return;

  // 殻はネットワーク優先。落ちていたらキャッシュで開く。
  // キャッシュ優先にすると、直したのに古い画面が出続ける。
  e.respondWith(
    fetch(e.request)
      .then((res) => {
        // 失敗の応答を殻として保存しない。500 のエラーページを掴むと、
        // 次に圏外で開いたときそれが画面として出続ける。
        if (res.ok) {
          const copy = res.clone();
          caches.open(SHELL).then((c) => c.put(e.request, copy));
        }
        return res;
      })
      .catch(() => caches.match(e.request).then((r) => r || Response.error())),
  );
});
