// この先の予定のページを実機で確かめる。
//
// 単体テストでは踏めない配線（今日からのリンク、開閉、戻るジェスチャー、
// オフライン表示、service worker がこの経路をキャッシュしていないこと）
// をここで見る。使い方は scripts/nav-check.mjs と同じ。ポートは APP= と
// API= で渡す。
const { chromium } = await import(process.env.PLAYWRIGHT ?? 'playwright-core');
const APP = process.env.APP ?? 'http://localhost:4173';
const TOKEN = 'dev-token-0123456789abcdef0123456789ab';
const results = [];
const check = (n, ok, d = '') => {
  results.push({ n, ok, d });
  console.log(`${ok ? '✓' : '✗'} ${n}${d ? ' — ' + d : ''}`);
};

// 静的な確認：useForecast が Outbox（送信の待ち行列）を使っていないこと。
// 実行時ではなくソースを直接見る。
//
// service worker が /api を扱っていないことは、静的な文字列チェック
// （sw.ts に "/api" が無いこと）では確かめられない。sw.ts には無関係な
// コメント（「同一オリジンに置いていた頃は /api を除外する分岐で守っ
// ていた」）が既にあり、それにヒットして常に落ちる。かわりに実機の
// CacheStorage を直接見る（下の「service worker が /api/sessions/
// forecast をキャッシュしていない」）。
const fs = await import('node:fs');
const hookSrc = fs.readFileSync(new URL('../src/features/forecast/useForecast.ts', import.meta.url), 'utf8');
check('useForecast が Outbox を使っていない', !hookSrc.includes('outbox'));

const browser = await chromium.launch({
  executablePath: process.env.CHROME ?? '/usr/bin/chromium',
  args: ['--no-sandbox'],
});
const page = await (await browser.newContext({ viewport: { width: 390, height: 844 } })).newPage();
const errs = [];
page.on('pageerror', (e) => errs.push('pageerror: ' + e.message));
page.on('console', (m) => {
  if (m.type() === 'error') errs.push(m.text());
});

await page.goto(`${APP}/#token=${TOKEN}`);
await page.waitForTimeout(2500);

// 初回訪問のページは service worker の制御下に無い
// （scripts/sw-update-check.mjs の教訓と同じ）。制御していない状態で
// キャッシュを覗いても「何もキャッシュしていない」が常に真になり、
// 何も守っていないチェックになる。一度読み直して制御下に入れてから
// 確かめる。
await page.evaluate(() => navigator.serviceWorker.ready.then(() => undefined));
await page.reload();
await page.waitForTimeout(2000);
check(
  'service worker がページを制御している（この先のキャッシュ確認の前提）',
  await page.evaluate(() => navigator.serviceWorker.controller !== null),
);

const body = () => page.innerText('body');
check('今日の画面にリンクがある', (await body()).includes('この先の予定を見る'));
check('タブには出ない', (await page.locator('nav').first().innerText()).includes('予定') === false);

await page.click('text=この先の予定を見る');
await page.waitForTimeout(1500);
const forecastBody = await body();
// 「この先の予定を見る」の一言だけでも「この先の予定」を含むので、
// クリックが何もしなくても通ってしまう。予定のページにしか無い
// 「← 今日」（戻るボタン）で見る。
check('予定のページが開く', forecastBody.includes('← 今日') && forecastBody.includes('この先の予定'));
check('今日は開いている', /▼\s*今日/.test(forecastBody));

// キャッシュ対象外の確認（実機）：この先の予定のデータを読み終えた
// あとに、どのキャッシュにも forecast の URL が入っていないこと。
// 陰性の確認なので、通信を1回消すだけでは判定できない——データが
// 実際に読み終わったこのタイミングで CacheStorage を直接列挙する。
// 列挙そのものが空では「キャッシュしていない」を証明したことにならない
// ので、まず「何かはキャッシュしている」（workbox の precache）ことを
// 確認してから、forecast の URL が含まれていないことを見る。
const { total, forecastHits } = await page.evaluate(async () => {
  const names = await caches.keys();
  const urls = [];
  for (const name of names) {
    const cache = await caches.open(name);
    for (const req of await cache.keys()) urls.push(req.url);
  }
  return { total: urls.length, forecastHits: urls.filter((u) => u.includes('/api/sessions/forecast')) };
});
check('service worker が何かをキャッシュしている（列挙が機能している証拠）', total > 0, `${total}件`);
check(
  'service worker が /api/sessions/forecast をキャッシュしていない',
  forecastHits.length === 0,
  forecastHits.join(', '),
);

// 折りたたみを開く
const collapsed = page.locator('button[aria-expanded="false"]').first();
if (await collapsed.count()) {
  await collapsed.click();
  await page.waitForTimeout(400);
  check('たたんだ回を開ける', (await collapsed.getAttribute('aria-expanded')) === 'true');
}

// 戻るジェスチャーで今日に帰る
await page.goBack();
await page.waitForTimeout(800);
check('戻るで今日に帰る', (await body()).includes('この先の予定を見る'));

// オフライン（見込みの通信だけを落とす）
await page.route('**/api/sessions/forecast*', (route) => route.abort());
await page.click('text=この先の予定を見る');
await page.waitForTimeout(1500);
check('オフラインで一言出る', (await body()).includes('オフラインでは見られません'));

await page.screenshot({ path: '/tmp/forecast.png' });

console.log('\nエラー:', errs.length ? errs.join('\n') : '(なし)');
const failed = results.filter((r) => !r.ok);
console.log(`\n${results.length - failed.length}/${results.length} 通過`);
await browser.close();
process.exit(failed.length ? 1 : 0);
