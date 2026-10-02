// 記録ボタンを押してから画面が進むまでの待ちを実機で確かめる。
//
// 押してから画面が進むのは、端末に積めた時点であるべきで、サーバーの応答を
// 待つべきではない。以前は積んだあとに送信の完了まで待っていたので、応答が
// 2 秒遅いとシートが閉じるまで 2 秒かかった（ジムの弱い電波で効く）。
// 送信そのものは裏で回り、最終的に1件だけ届くことも見る。
//
// サーバーの応答の遅れは page.route で作る。ネットワークの遅さそのものを
// 再現できるので、CPU を絞るより決まった結果になる。
//
// 使い方は scripts/ui-check.mjs と同じ。ポートは APP= と API= で渡す。
const { chromium } = await import(process.env.PLAYWRIGHT ?? 'playwright-core');
const APP = process.env.APP ?? 'http://localhost:4173';
const API = process.env.API ?? 'http://127.0.0.1:8080';
const TOKEN = 'dev-token-0123456789abcdef0123456789ab';
const results = [];
const check = (n, ok, d = '') => {
  results.push({ n, ok, d });
  console.log(`${ok ? '✓' : '✗'} ${n}${d ? ' — ' + d : ''}`);
};

const day = () => {
  const d = new Date();
  const p = (n) => String(n).padStart(2, '0');
  return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())}`;
};
const auth = { Authorization: `Bearer ${TOKEN}` };
const todaysSets = () =>
  fetch(`${API}/api/set-logs?from=${day()}&to=${day()}`, { headers: auth })
    .then((r) => r.json())
    .then((b) =>
      (b.days ?? []).flatMap((d) =>
        d.exercises.flatMap((e) => e.sets.map((s) => ({ ...s, exercise_id: e.exercise_id }))),
      ),
    );
const clearToday = async () => {
  for (const x of await todaysSets()) {
    await fetch(`${API}/api/set-logs/${encodeURIComponent(x.id)}`, { method: 'DELETE', headers: auth });
  }
};

const browser = await chromium.launch({
  executablePath: process.env.CHROME ?? '/usr/bin/chromium',
  args: ['--no-sandbox'],
});
const errs = [];
const start = async (postDelayMs) => {
  await clearToday();
  const page = await (
    await browser.newContext({ viewport: { width: 390, height: 844 }, deviceScaleFactor: 2 })
  ).newPage();
  page.on('pageerror', (e) => errs.push('pageerror: ' + e.message));
  page.on('console', (m) => {
    if (m.type() === 'error') errs.push(m.text());
  });
  await page.goto(`${APP}/?t=${Date.now()}#token=${TOKEN}`);
  await page.waitForTimeout(2500);
  if (postDelayMs) {
    await page.route('**/api/set-logs', async (route) => {
      if (route.request().method() === 'POST') await new Promise((r) => setTimeout(r, postDelayMs));
      await route.continue();
    });
  }
  return page;
};
const fill = async (page, slot, w, r) => {
  await page.locator('button.set').nth(slot).click();
  await page.waitForTimeout(400);
  await page.fill('dialog input[inputmode=decimal]', String(w));
  await page.fill('dialog input[inputmode=numeric] >> nth=0', String(r));
};
const tapAndTime = async (page) => {
  const t0 = Date.now();
  await page.click('dialog >> text=記録する');
  await page.waitForFunction(() => !document.querySelector('dialog[open]'), null, { timeout: 15000 });
  return Date.now() - t0;
};

// 1. 応答が2秒遅くても、押した直後に画面が進む。
{
  const page = await start(2000);
  await fill(page, 0, 100, 5);
  const ms = await tapAndTime(page);
  check('応答が2秒遅くても、シートは 400ms 以内に閉じる', ms < 400, `${ms}ms`);
  check('閉じた時点で、枠が記録済みになっている', (await page.locator('button.set-done').count()) === 1);
  check('送信はまだ終わっていない（応答待ちの最中）', (await todaysSets()).length === 0);
  await page.waitForTimeout(3500);
  const sets = await todaysSets();
  check('送信が済むと、サーバーに1セット届く', sets.length === 1, `${sets.length}セット`);
  await page.close();
}

// 2. 回線が速いときも従来どおり。
{
  const page = await start(0);
  await fill(page, 0, 100, 5);
  const ms = await tapAndTime(page);
  check('回線が速いときも、シートは 400ms 以内に閉じる', ms < 400, `${ms}ms`);
  await page.waitForTimeout(1500);
  check('回線が速いとき、サーバーに1セット届く', (await todaysSets()).length === 1);
  await page.close();
}

check('コンソールにエラーが出ない', errs.length === 0, errs.join(' | '));
await clearToday();
const failed = results.filter((r) => !r.ok);
console.log(`\n${results.length - failed.length}/${results.length} 通過`);
await browser.close();
process.exit(failed.length ? 1 : 0);
