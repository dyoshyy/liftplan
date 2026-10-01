// 記録を素早く連打しても1回分しか記録されないことを実機で確かめる。
//
// 単体テストでは踏めないところを見る。二重に記録される原因は、記録の手順が
// 非同期（待ち行列への書き込みを待つ）で、待っている間もシートが開いたままで
// 「記録する」が押せること。React の状態も <dialog> も絡むので、DOM を
// 立てないと再現できない。
//
// 使い方は scripts/ui-check.mjs と同じ。ポートは APP= と API= で渡す。
const { chromium } = await import(process.env.PLAYWRIGHT ?? 'playwright-core');
const APP = process.env.APP ?? 'http://localhost:4173';
const TOKEN = 'dev-token-0123456789abcdef0123456789ab';
const results = [];
const check = (n, ok, d = '') => {
  results.push({ n, ok, d });
  console.log(`${ok ? '✓' : '✗'} ${n}${d ? ' — ' + d : ''}`);
};

const API = process.env.API ?? 'http://127.0.0.1:8080';
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
await clearToday();

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

await page.goto(`${APP}/#token=${TOKEN}`);
await page.waitForTimeout(2500);

const open = async (slot, w, r) => {
  await page.locator('button.set').nth(slot).click();
  await page.waitForTimeout(400);
  await page.fill('dialog input[inputmode=decimal]', String(w));
  await page.fill('dialog input[inputmode=numeric] >> nth=0', String(r));
};
const settle = () => page.waitForTimeout(2500);

// 1. 同じ tick に2回押す（最悪の連打）。
await open(0, 100, 5);
await page.evaluate(() => {
  const b = [...document.querySelectorAll('dialog button')].find((x) => x.textContent.includes('記録する'));
  b.click();
  b.click();
});
await settle();
let sets = await todaysSets();
check('同時に2回押しても、サーバーに届くのは1セット', sets.length === 1, `${sets.length}セット`);
const doneSlots = await page.locator('button.set-done').count();
check('画面の記録済みの枠も1つ', doneSlots === 1, `${doneSlots}枠`);

// 2. 少し間を置いて2回押す（人の連打）。
await clearToday();
await page.reload();
await page.waitForTimeout(2500);
await open(0, 100, 5);
await page.evaluate(async () => {
  const b = [...document.querySelectorAll('dialog button')].find((x) => x.textContent.includes('記録する'));
  b.click();
  await new Promise((r) => setTimeout(r, 15));
  b.click();
});
await settle();
sets = await todaysSets();
check('15ms 空けて2回押しても、サーバーに届くのは1セット', sets.length === 1, `${sets.length}セット`);

// 2b. 遅い端末で、人が連打する間隔（40ms）で2回押す。
//
// スマホは待ち行列への書き込み（IndexedDB）と描画が遅く、シートが閉じる前に
// 2回目が押せる。CPU を遅くして、その状況を作る。
const cdp = await page.context().newCDPSession(page);
await clearToday();
await page.reload();
await page.waitForTimeout(2500);
await open(0, 100, 5);
await cdp.send('Emulation.setCPUThrottlingRate', { rate: 20 });
await page.evaluate(async () => {
  const b = [...document.querySelectorAll('dialog button')].find((x) => x.textContent.includes('記録する'));
  b.click();
  await new Promise((r) => setTimeout(r, 40));
  if (b.isConnected) b.click();
});
await settle();
await cdp.send('Emulation.setCPUThrottlingRate', { rate: 1 });
await page.waitForTimeout(1500);
sets = await todaysSets();
check(
  '遅い端末で40ms空けて2回押しても、サーバーに届くのは1セット',
  sets.length === 1,
  `${sets.length}セット`,
);

// 3. 取り消しの連打。
await clearToday();
await page.reload();
await page.waitForTimeout(2500);
await open(0, 100, 5);
await page.click('dialog >> text=記録する');
await page.waitForTimeout(900);
await page.locator('button.set').nth(0).click();
await page.waitForTimeout(400);
await page.evaluate(() => {
  const b = [...document.querySelectorAll('dialog button')].find((x) =>
    x.textContent.includes('この記録を取り消す'),
  );
  b.click();
  b.click();
});
await settle();
sets = await todaysSets();
check('取り消しを連打しても、記録は消えている', sets.length === 0, `${sets.length}セット`);

// 4. 通常の記録は連打しなくても1回で通る（ガードが記録を止めていないこと）。
await clearToday();
await page.reload();
await page.waitForTimeout(2500);
for (const [slot, w, r] of [
  [0, 100, 5],
  [1, 100, 5],
  [2, 97.5, 6],
]) {
  await open(slot, w, r);
  await page.click('dialog >> text=記録する');
  await page.waitForTimeout(900);
}
await settle();
sets = await todaysSets();
check(
  '続けて3セット記録すると3セット届く（ガードが次の記録を止めない）',
  sets.length === 3,
  `${sets.length}セット`,
);

check('コンソールにエラーが出ない', errs.length === 0, errs.join(' | '));
await clearToday();
const failed = results.filter((r) => !r.ok);
console.log(`\n${results.length - failed.length}/${results.length} 通過`);
await browser.close();
process.exit(failed.length ? 1 : 0);
