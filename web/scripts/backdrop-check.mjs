// シートの暗い部分を押すと閉じることを実機で確かめる。
//
// 判断（押した先と離した先がどちらも <dialog> か）は ui/backdrop.ts の単体テストが
// 守っている。ここは配線を見る。3つのシートに付いているか、<dialog> の余白が 0 の
// ままで中身を押しても閉じないか、中で押して外で離したときに閉じないか。
//
// 使い方は scripts/ui-check.mjs と同じ。ポートは APP= と API= で渡す。
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

const day = () => {
  const d = new Date();
  const p = (n) => String(n).padStart(2, '0');
  return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())}`;
};
const clearToday = async () => {
  const b = await fetch(`${API}/api/set-logs?from=${day()}&to=${day()}`, { headers: auth }).then((r) =>
    r.json(),
  );
  for (const s of (b.days ?? []).flatMap((d) => d.exercises.flatMap((e) => e.sets))) {
    await fetch(`${API}/api/set-logs/${encodeURIComponent(s.id)}`, { method: 'DELETE', headers: auth });
  }
};
await clearToday();

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

const dialog = page.locator('dialog[open]');
const isOpen = async () => (await dialog.count()) === 1;
// 画面の一番上はどのシートにも覆われない（<dialog> の max-height が上に余白を残す）。
const tapBackdrop = async () => {
  await page.mouse.click(195, 8);
  await page.waitForTimeout(300);
};

await page.goto(`${APP}/#token=${TOKEN}`);
await page.waitForTimeout(2500);

const recorded = await page.locator('span.font-bold.text-\\[17px\\]').first().innerText();
await page.locator('button.set').first().click();
await page.waitForTimeout(400);
check('前提: 記録シートが開く', await isOpen());
await tapBackdrop();
check('記録シート: 暗い部分を押すと閉じる', !(await isOpen()));

await page.locator('button.set').first().click();
await page.waitForTimeout(400);
await dialog.locator('span.font-bold').first().click();
await page.waitForTimeout(300);
check('記録シート: 中を押しても閉じない', await isOpen());

const input = await dialog.locator('input').first().boundingBox();
await page.mouse.move(input.x + input.width / 2, input.y + input.height / 2);
await page.mouse.down();
await page.mouse.move(195, 8, { steps: 5 });
await page.mouse.up();
await page.waitForTimeout(300);
check('記録シート: 中で押して暗い部分で離しても閉じない', await isOpen());

await page.fill('dialog input[inputmode=decimal]', '40');
await page.fill('dialog input[inputmode=numeric] >> nth=0', '10');
await page.click('dialog >> text=記録する');
await page.waitForTimeout(900);
check('前提: 記録すると閉じる', !(await isOpen()));

await page.getByRole('button', { name: '種目を選んで記録' }).click();
await page.waitForTimeout(400);
check('前提: 種目を選ぶシートが開く', await isOpen());
await tapBackdrop();
check('種目を選ぶシート: 暗い部分を押すと閉じる', !(await isOpen()));

await page.click('nav >> text=履歴');
await page.waitForTimeout(1500);
await page
  .getByRole('button', { name: new RegExp(`^${recorded}`) })
  .first()
  .click();
await page.waitForTimeout(300);
await page.getByRole('button', { name: `${recorded} 1セット目を直す` }).click();
await page.waitForTimeout(400);
check('前提: 記録を直すシートが開く', await isOpen());
await tapBackdrop();
check('記録を直すシート: 暗い部分を押すと閉じる', !(await isOpen()));

check('コンソールにエラーが出ない', errs.length === 0, errs.join(' | '));

await clearToday();
const failed = results.filter((r) => !r.ok);
console.log(`\n${results.length - failed.length}/${results.length} 通過`);
await browser.close();
process.exit(failed.length ? 1 : 0);
