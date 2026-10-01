// 休憩タイマーと記録シートの初期値を実機で確かめる。
//
// 単体テストでは踏めないところを見る。タイマーは時間と localStorage と
// 再読み込みをまたぐので、純粋な関数の検査だけでは「画面で本当に動くか」が
// 分からない。初期値も、シートを開いたときに何が入るかは DOM でしか見えない。
//
// **本番ビルドを preview で出して使う。**使い方:
//
//   # サーバー。DATABASE_URL を付けないこと。DEV_SESSION_TOKEN は
//   # インメモリ構成でしか効かず、付けると検査が全部 401 になる。
//   # OAuth の4つは起動の条件なので値が要るが、検査はログインを
//   # 通らないので中身は何でもよい。AUTH_TOKEN は廃止した（残っていると
//   # 起動を拒む）。
//   cd .. && DEV_SESSION_TOKEN=dev-token-0123456789abcdef0123456789ab \
//     API_ORIGIN=http://localhost:8080 WEB_ORIGIN=http://localhost:4173 \
//     ALLOWED_ORIGINS=http://localhost:4173 \
//     GITHUB_CLIENT_ID=dev GITHUB_CLIENT_SECRET=dev \
//     GOOGLE_CLIENT_ID=dev GOOGLE_CLIENT_SECRET=dev \
//     PORT=8080 go run ./cmd/api
//
//   # 画面
//   ALLOW_LOCAL_API=1 VITE_API_BASE=http://127.0.0.1:8080 pnpm build
//   pnpm exec vite preview --port 4173 --strictPort
//
//   # 検証（ポートを変えるなら APP= と API= で渡す）
//   PLAYWRIGHT=<playwright-core のパス> node scripts/ui-check.mjs
//
// CI には入れていない。ブラウザの実体が要るため。手で回す。
const { chromium } = await import(process.env.PLAYWRIGHT ?? 'playwright-core');
const APP = process.env.APP ?? 'http://localhost:4173';
const TOKEN = 'dev-token-0123456789abcdef0123456789ab';

const results = [];
const check = (n, ok, d = '') => {
  results.push({ n, ok, d });
  console.log(`${ok ? '✓' : '✗'} ${n}${d ? ' — ' + d : ''}`);
};

// 前回の実行が残した記録を消す。
//
// 残っていると1セット目が「新規」ではなく「修正」になり、仕様どおり
// タイマーが始まらない。検査が壊れているのか実装が壊れているのか
// 区別できなくなる。
const API = process.env.API ?? 'http://127.0.0.1:8080';
const today = () => {
  const d = new Date();
  const p = (n) => String(n).padStart(2, '0');
  return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())}`;
};
const auth = { Authorization: `Bearer ${TOKEN}` };
const existing = await fetch(`${API}/api/set-logs?from=${today()}&to=${today()}`, { headers: auth })
  .then((r) => r.json())
  .then((b) => (b.days ?? []).flatMap((d) => d.exercises.flatMap((e) => e.sets)));
for (const s of existing) {
  await fetch(`${API}/api/set-logs/${encodeURIComponent(s.id)}`, { method: 'DELETE', headers: auth });
}
console.log(`（前回の記録を ${existing.length} 件消した）\n`);

const browser = await chromium.launch({
  executablePath: process.env.CHROME ?? '/usr/bin/chromium',
  args: ['--no-sandbox'],
});
const page = await (
  await browser.newContext({ viewport: { width: 390, height: 844 }, deviceScaleFactor: 2 })
).newPage();
const errs = [];
page.on('pageerror', (e) => errs.push('pageerror: ' + e.message));
page.on('console', (m) => {
  if (m.type() === 'error') errs.push(m.text());
});

// ログインはフラグメントで済ませる。/auth/* を通すとプロバイダの画面が
// 挟まり、自動では抜けられない。#token= はサーバーがコールバックで戻して
// くる形そのものなので、取り込みの配線もここで一緒に検査できる。
await page.goto(`${APP}/#token=${TOKEN}`);
await page.waitForTimeout(2500);

// 休憩バーは休憩中だけ出る（PR #78）。待機中は存在しないので、
// 「出ていないこと」から確かめる。
const bar = () => page.locator('div.fixed.z-40');
const barText = async () => ((await bar().count()) ? bar().innerText() : '');

check('待機中は休憩バーが出ていない', (await bar().count()) === 0);

const record = async (i, w, r) => {
  await page.locator('button.set').nth(i).click();
  await page.waitForTimeout(400);
  await page.fill('dialog input[inputmode=decimal]', String(w));
  await page.fill('dialog input[inputmode=numeric] >> nth=0', String(r));
  await page.click('dialog >> text=記録する');
  await page.waitForTimeout(900);
};

await record(0, 102.5, 6);

check('記録で休憩が始まる', /休憩中/.test(await barText()), (await barText()).replace(/\s+/g, ' '));
check('既定は3分', (await barText()).includes('3:00'), (await barText()).replace(/\s+/g, ' '));

// 秒は切り上げるので、記録直後はまだ 3:00 のまま。1秒以上待って減ることを見る。
await page.waitForTimeout(1500);
check('カウントダウンしている', !(await barText()).includes('3:00'), (await barText()).replace(/\s+/g, ' '));

// 2セット目の初期値が、今日の直前のセットに揃うこと。
await page.locator('button.set').nth(1).click();
await page.waitForTimeout(500);
const w2 = await page.inputValue('dialog input[inputmode=decimal]');
const r2 = await page.inputValue('dialog input[inputmode=numeric] >> nth=0');
check('2セット目の重量が直前のセットに揃う', w2 === '102.5', `重量=${w2}`);
check('2セット目のレップが直前のセットに揃う', r2 === '6', `レップ=${r2}`);
await page.click('dialog >> text=閉じる');
await page.waitForTimeout(400);

// 一時停止しているあいだは減らない。
await page.click('text=一時停止');
await page.waitForTimeout(1200);
const p1 = (await barText()).match(/\d:\d\d/)[0];
await page.waitForTimeout(2000);
const p2 = (await barText()).match(/\d:\d\d/)[0];
check('止めているあいだは減らない', p1 === p2, `${p1} → ${p2}`);

// やめると消える。
await page.click('text=やめる');
await page.waitForTimeout(500);
check('やめると休憩バーが消える', (await bar().count()) === 0);

// 長さは設定画面で変え、再読み込みをまたいで残ること。
await page.click('header button[aria-label="設定"]');
await page.waitForTimeout(1500);
// 節は畳んである。開いてからステッパーを触る。
await page.click('text=休憩の長さ');
await page.waitForTimeout(500);
// 休憩の長さのステッパーを指す。設定画面には週目標の数値入力も並ぶので、
// input[type=number] の先頭を取ると別のものを掴む。
const stepper = page.locator('div:has(> button[aria-label*="減らす"])').first();
const minus = stepper.locator('button[aria-label*="減らす"]');
await minus.click();
await minus.click();
await minus.click();
await minus.click();
await page.waitForTimeout(500);

await page.reload();
await page.waitForTimeout(2500);
await page.click('header button[aria-label="設定"]');
await page.waitForTimeout(1500);
await page.click('text=休憩の長さ');
await page.waitForTimeout(500);
const kept = await page
  .locator('div:has(> button[aria-label*="減らす"])')
  .first()
  .locator('input[type=number]')
  .inputValue();
check('休憩の長さが再読み込みをまたいで残る', kept === '2', `長さ=${kept}分`);

await page.screenshot({ path: '/tmp/ui-check.png' });
console.log('\nエラー:', errs.length ? errs.join('\n') : '(なし)');
const failed = results.filter((r) => !r.ok);
console.log(`\n${results.length - failed.length}/${results.length} 通過`);
await browser.close();
process.exit(failed.length ? 1 : 0);
