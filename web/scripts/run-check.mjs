// ランの記録を実機で確かめる。
//
// 単体テストは「何を積むか」（planRun・planUndoRun）と「何を出すか」
// （visibleRuns・parseRunInput）しか見ていない。「今日」に入口が出るか、
// シートの入力が parseRunInput に届いて記録を押せるようになるか、積んだものが
// サーバーに届くか、開き直したときにサーバーから読んで1件だけ出るか、
// 取り消しが届くかは配線なので、ここで見る。
//
// 使い方は scripts/ui-check.mjs と同じ。ポートは APP= と API= で渡す。
// 始めと終わりに今日のランを消す。
const { chromium } = await import(process.env.PLAYWRIGHT ?? 'playwright-core');
const APP = process.env.APP ?? 'http://localhost:4173';
const API = process.env.API ?? 'http://127.0.0.1:8080';
const TOKEN = 'dev-token-0123456789abcdef0123456789ab';
const auth = { Authorization: `Bearer ${TOKEN}` };
const results = [];
const check = (n, ok, d = '') => {
  results.push({ n, ok, d });
  console.log(`${ok ? '✓' : '✗'} ${n}${d ? ' — ' + d : ''}`);
};

const p2 = (n) => String(n).padStart(2, '0');
const now = new Date();
const TODAY = `${now.getFullYear()}-${p2(now.getMonth() + 1)}-${p2(now.getDate())}`;

const runsToday = () =>
  fetch(`${API}/api/run-logs?from=${TODAY}&to=${TODAY}`, { headers: auth })
    .then((r) => r.json())
    .then((b) => b.runs ?? []);
const clear = async () => {
  for (const r of await runsToday()) {
    await fetch(`${API}/api/run-logs/${encodeURIComponent(r.id)}`, { method: 'DELETE', headers: auth });
  }
};
const until = async (pred, ms = 8000) => {
  const end = Date.now() + ms;
  for (;;) {
    const v = await pred();
    if (v || Date.now() > end) return v;
    await new Promise((r) => setTimeout(r, 300));
  }
};

await clear();

const browser = await chromium.launch({
  executablePath: process.env.CHROME ?? '/usr/bin/chromium',
  args: ['--no-sandbox'],
});
const context = await browser.newContext({ viewport: { width: 390, height: 844 } });
const page = await context.newPage();
const errs = [];
page.on('pageerror', (e) => errs.push('pageerror: ' + e.message));
page.on('console', (m) => {
  if (m.type() === 'error') errs.push(m.text());
});

const dialog = page.locator('dialog[open]');
const entry = page.getByRole('button', { name: 'ランを記録', exact: true });
const runLines = () => page.locator('text=/^\\d+(\\.\\d+)?km \\d+:\\d{2}(:\\d{2})?$/');

await page.goto(`${APP}/#token=${TOKEN}`);
await entry.waitFor({ timeout: 10000 });
check('「今日」にランを記録の入口がある', (await entry.count()) === 1);

await entry.click();
const submit = dialog.getByRole('button', { name: '記録する', exact: true });
check('シートが開く', (await dialog.count()) === 1);
check('何も入れていなければ記録を押せない', await submit.isDisabled());

await dialog.getByLabel('距離 (km)').fill('5.2');
await dialog.getByLabel('分').fill('28');
await dialog.getByLabel('秒').fill('30');
check('距離と時間を入れたら記録を押せる', await submit.isEnabled());

await submit.click();
await page.waitForTimeout(300);
check('記録したらシートが閉じる', (await dialog.count()) === 0);
check('記録したランが行に出る', (await page.locator('text=5.2km 28:30').count()) === 1);

const sent = await until(async () => {
  const runs = await runsToday();
  return runs.length === 1 ? runs : null;
});
check(
  'サーバーに届く',
  sent?.[0]?.distance_km === 5.2 && sent?.[0]?.duration_seconds === 28 * 60 + 30,
  JSON.stringify(sent),
);

await page.reload();
await entry.waitFor({ timeout: 10000 });
await page.waitForTimeout(1000);
check(
  '開き直してもサーバーから読んで1件だけ出る',
  (await runLines().count()) === 1,
  String(await runLines().count()),
);

await page.getByRole('button', { name: '取り消す', exact: true }).click();
await page.waitForTimeout(300);
check('取り消したら行から消える', (await runLines().count()) === 0);
check('取り消しがサーバーに届く', await until(async () => (await runsToday()).length === 0));

await page.screenshot({ path: '/tmp/run-check.png' });
console.log('\nエラー:', errs.length ? errs.join('\n') : '(なし)');
check('コンソールにエラーが出ない', errs.length === 0, errs.join(' | '));

await clear();
const failed = results.filter((r) => !r.ok);
console.log(`\n${results.length - failed.length}/${results.length} 通過`);
await browser.close();
process.exit(failed.length ? 1 : 0);
