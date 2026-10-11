// 初回の案内を実機で確かめる。
//
// 単体テストは送る順（onboarding.ts）と段階の進み方（coach.ts）しか見て
// いない。設定画面が「今日」の代わりに出ること、「はじめる」で本当に保存
// されること、コーチマークが本物の画面の要素を指して出ること、再読み込みで
// 二度と出ないことは配線なので、ここで見る。
//
// **API は DEV_ONBOARDING_PENDING=1 で立てる。**無いと開発用の利用者は
// 済んだ状態で始まり、設定画面が出ない（1で止まる）。
//
// 使い方は scripts/ui-check.mjs と同じ。ポートは APP= と API= で渡す。
const { chromium } = await import(process.env.PLAYWRIGHT ?? 'playwright-core');
const APP = process.env.APP ?? 'http://localhost:4173';
const API = process.env.API ?? 'http://127.0.0.1:8080';
const SHOTS = process.env.SHOTS; // スクリーンショットを置くディレクトリ。無ければ撮らない
const TOKEN = 'dev-token-0123456789abcdef0123456789ab';
const auth = { Authorization: `Bearer ${TOKEN}` };
const get = (path) => fetch(`${API}${path}`, { headers: auth }).then((r) => r.json());

let failed = 0;
const check = (ok, label) => {
  console.log(`${ok ? 'OK  ' : 'NG  '} ${label}`);
  if (!ok) failed++;
};

const before = await get('/api/onboarding');
check(before.completed === false, `始めは未完了（completed=${before.completed}）`);
if (before.completed) {
  console.log('API を DEV_ONBOARDING_PENDING=1 で立て直すこと');
  process.exit(1);
}

const browser = await chromium.launch({
  executablePath: process.env.CHROME ?? '/usr/bin/chromium',
  args: ['--no-sandbox'],
});
const page = await (await browser.newContext({ viewport: { width: 390, height: 844 } })).newPage();
const errs = [];
page.on('pageerror', (e) => errs.push(e.message));
page.on('console', (m) => {
  if (m.type() === 'error') errs.push(m.text());
});
const shot = async (name) => {
  if (SHOTS) await page.screenshot({ path: `${SHOTS}/onboarding-${name}.png` });
};

// 0. 設定画面の読み込みが1回失敗しても抜けられる。歯車も下のナビも
//    出していないので、読み直す手段が画面に無いと骨組みのまま止まる。
//    プリセットは loadAll が取らないので、ここで初めて読まれる。
let failPresets = true;
await page.route('**/api/split-presets', (route) => {
  if (!failPresets) return route.continue();
  failPresets = false;
  return route.abort();
});
await page.goto(`${APP}/#token=${TOKEN}`);
const FREQ = 'select[aria-label="週に通う回数"]';
const retry = page.getByRole('button', { name: '読み直す' });
const retried = await retry
  .waitFor({ timeout: 10000 })
  .then(() => true)
  .catch(() => false);
check(retried, '読み込みに失敗したら「読み直す」が出る');
await shot('0-load-failed');
if (retried) await retry.click();
await page.waitForSelector(FREQ, { timeout: 10000 });
await shot('1-frequency');

// 1. 「今日」の代わりに出ていて、抜け道（歯車・下のナビ）が無い
check((await page.locator('header button[aria-label="設定"]').count()) === 0, '歯車が出ていない');
check((await page.getByRole('navigation').count()) === 0, '下のナビが出ていない');
check((await page.getByText('休憩を始める').count()) === 0, '「今日」が出ていない');

// 2. 週5・5分割を選んで進む
await page.selectOption(FREQ, '5');
await page.getByRole('button', { name: '次へ' }).click();
const fiveWay = page.getByRole('button', { name: /^5分割/ });
check(await fiveWay.isEnabled(), '週5なら5分割を選べる');
await fiveWay.click();
await shot('2-split');
await page.getByRole('button', { name: '次へ' }).click();
await shot('3-volume');
await page.getByRole('button', { name: '次へ' }).click();
await shot('4-declared');

// 戻っても選んだ値が残る（段階2で5分割が押された状態）
await page.getByRole('button', { name: '戻る' }).click();
await page.getByRole('button', { name: '戻る' }).click();
check((await fiveWay.getAttribute('aria-pressed')) === 'true', '戻っても5分割が選ばれたまま');
await page.getByRole('button', { name: '次へ' }).click();
await page.getByRole('button', { name: '次へ' }).click();
await page.getByRole('button', { name: 'はじめる' }).click();

// 3. 保存されている
const dialog = page.getByRole('dialog', { name: '使い方' });
await dialog.waitFor({ timeout: 10000 });
const program = await get('/api/program');
check(program.per_week === 5, `per_week=5（${program.per_week}）`);
check(program.splits.length === 5, `分割が5件（${program.splits.length}）`);
check((await get('/api/onboarding')).completed === true, '完了が記録されている');

// 4. コーチマークが出て（上の waitFor）、最後まで進められる
const texts = [];
for (let i = 0; i < 10; i++) {
  texts.push(await dialog.locator('p').innerText());
  await shot(`5-coach-${i + 1}`);
  const done = dialog.getByRole('button', { name: 'はじめる' });
  if (await done.count()) {
    await done.click();
    break;
  }
  await dialog.getByRole('button', { name: '次へ' }).click();
}
texts.forEach((t, i) => console.log(`     ${i + 1}: ${t}`));
check(texts.length >= 3, `段階が3つ以上出た（${texts.length}）`);
check((await dialog.count()) === 0, '最後で閉じた');
check(await page.getByText('休憩を始める').isVisible(), '「今日」が出ている');

// 5. 再読み込みで二度と出ない
await page.reload();
await page.waitForTimeout(2500);
check((await page.locator(FREQ).count()) === 0, '再読み込みで設定画面が出ない');
check((await dialog.count()) === 0, '再読み込みでコーチマークが出ない');
await shot('6-reloaded');

// 0 で落とした読み込みの1件は除く。
const unexpected = errs.filter((e) => !e.includes('ERR_FAILED'));
check(unexpected.length === 0, `画面のエラーが0件（${unexpected.length}）`);
unexpected.forEach((e) => console.log('     ', e));

await browser.close();
console.log(failed === 0 ? '\n全部通った' : `\n${failed}件落ちた`);
process.exit(failed === 0 ? 0 : 1);
