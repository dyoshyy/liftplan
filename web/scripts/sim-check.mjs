// シミュレーション画面を実機で確かめる。
//
// 単体テストが見ていないのは**配線**。判断（simulate.ts）は緑でも、
// 問い合わせにトークンが載っていなければ画面は 401 しか出さない。ここは
// #108 から変わった部分そのもので（認証の外→内、ビルドの外→中）、
// 壊れても Go のテストも vitest も緑のまま通る。
//
// **本番ビルドに対して回す。**`pnpm dev` では index.html 以外の入力が
// 出力に入るかを検査できない（dev サーバーは dist を見ない）。
//
//   VITE_API_BASE=http://localhost:8080 pnpm build && pnpm preview
//   PLAYWRIGHT=<playwright-core のパス> node scripts/sim-check.mjs
//
// サーバー側は README の「シミュレーション画面」の手順で立てる
// （DEV_SESSION_TOKEN を下の TOKEN と揃えること）。
const { chromium } = await import(process.env.PLAYWRIGHT ?? 'playwright-core');
const APP = process.env.APP ?? 'http://localhost:4173';
const TOKEN = process.env.TOKEN ?? 'test-token-0123456789abcdef0123456789abcdef';
const results = [];
const check = (n, ok, d = '') => { results.push({ n, ok, d }); console.log(`${ok ? '✓' : '✗'} ${n}${d ? ' — ' + d : ''}`); };

// 1. ビルドに入っていること。入力を1つに戻すと preview が index.html を
//    返すので、中身まで見ないと「200 が返った」で騙される。
const html = await fetch(`${APP}/dev.html`).then((r) => r.text());
check('dev.html がビルドに入っている', html.includes('/src/dev/main.tsx') || /assets\/dev-[^"]+\.js/.test(html), html.slice(0, 80).replace(/\s+/g, ' '));

// 2. precache に入っていないこと。入ると全端末の更新に乗り、しかも
//    更新の合図を押すまで古い版が出続ける。
const sw = await fetch(`${APP}/sw.js`).then((r) => r.text());
check('precache に入っていない', !sw.includes('dev.html') && !/assets\/dev-/.test(sw));

const browser = await chromium.launch({ executablePath: process.env.CHROME ?? '/usr/bin/chromium', args: ['--no-sandbox'] });
const context = await browser.newContext({ viewport: { width: 390, height: 844 }, deviceScaleFactor: 2 });
const page = await context.newPage();
const errs = [];
page.on('pageerror', (e) => errs.push('pageerror: ' + e.message));
page.on('console', (m) => { if (m.type() === 'error') errs.push(m.text()); });

const body = () => page.innerText('body');

// 3. トークンが無ければログインの案内が出ること。
//
// **「計画が出ない」では足りない。**サーバーが落ちていても出ないので、
// 401 を 401 として読めているかを文面で見る。
await page.goto(`${APP}/dev.html`);
await page.waitForTimeout(2500);
check('トークンが無いとログインへ案内する', (await body()).includes('ログイン'), (await body()).replace(/\s+/g, ' ').slice(0, 120));

// 4. トークンがあれば計画が出ること。取り込みはメインの画面と同じ
//    localStorage を読む（同一オリジン）ので、本物と同じ入れ方をする。
await page.goto(`${APP}/#token=${TOKEN}`);
await page.waitForTimeout(2000);
await page.goto(`${APP}/dev.html`);
await page.waitForTimeout(3000);
const shown = await body();
check('計画が出る', /セット|kg|自分で決める/.test(shown), shown.replace(/\s+/g, ' ').slice(0, 120));
check('ログインの案内は消える', !shown.includes('ログインが切れている'));

await page.screenshot({ path: '/tmp/sim-check.png', fullPage: true });

console.log('\nエラー:', errs.length ? errs.join('\n') : '(なし)');
const failed = results.filter((r) => !r.ok);
console.log(`\n${results.length - failed.length}/${results.length} 通過`);
await browser.close();
process.exit(failed.length ? 1 : 0);
