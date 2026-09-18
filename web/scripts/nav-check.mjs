// ナビゲーションを実機で確かめる。
//
// 単体テストでは踏めないところを見る。**戻るジェスチャーは history と
// popstate が絡むので、DOM を立てないと検査できない**。ここを落とすと
// 設定画面で戻ったときにアプリごと閉じる。
//
// 使い方は scripts/ui-check.mjs と同じ。ポートは APP= と API= で渡す。
const { chromium } = await import(process.env.PLAYWRIGHT ?? 'playwright-core');
const APP = process.env.APP ?? 'http://localhost:4173';
const TOKEN = 'dev-token-0123456789abcdef0123456789ab';
const results = [];
const check = (n, ok, d = '') => { results.push({ n, ok, d }); console.log(`${ok ? '✓' : '✗'} ${n}${d ? ' — ' + d : ''}`); };

// 前回の実行が残した記録を消す。残っていると1セット目が「修正」になり、
// 仕様どおりタイマーが始まらない（修正では走らせない）。
const API = process.env.API ?? 'http://127.0.0.1:8080';
const day = () => {
  const d = new Date();
  const p = (n) => String(n).padStart(2, '0');
  return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())}`;
};
const auth = { Authorization: `Bearer ${TOKEN}` };
const old = await fetch(`${API}/api/set-logs?from=${day()}&to=${day()}`, { headers: auth })
  .then((r) => r.json())
  .then((b) => (b.days ?? []).flatMap((d) => d.exercises.flatMap((e) => e.sets)));
for (const x of old) {
  await fetch(`${API}/api/set-logs/${encodeURIComponent(x.id)}`, { method: 'DELETE', headers: auth });
}
console.log(`（前回の記録を ${old.length} 件消した）\n`);

const browser = await chromium.launch({ executablePath: process.env.CHROME ?? '/usr/bin/chromium', args: ['--no-sandbox'] });
const page = await (await browser.newContext({ viewport: { width: 390, height: 844 }, deviceScaleFactor: 2 })).newPage();
const errs = [];
page.on('pageerror', (e) => errs.push('pageerror: ' + e.message));
page.on('console', (m) => { if (m.type() === 'error') errs.push(m.text()); });

await page.goto(APP);
await page.fill('input[type=password]', TOKEN);
await page.click('text=保存する');
await page.waitForTimeout(2500);

const body = () => page.innerText('body');
const navCount = () => page.locator('nav').count();

check('ナビが1本出る', (await navCount()) === 1);
check('待機中はタイマーバーが出ない', !(await body()).includes('休憩中'));
check('体重が1行になっている', (await page.locator('label[for=bw]').count()) === 1);
check('設定は今日の画面に無い', !(await body()).includes('週に通う回数'));

// 履歴タブ
await page.click('nav >> text=履歴');
await page.waitForTimeout(1800);
check('履歴が開く', /今週の充足|記録がまだ|推定1RM/.test(await body()), (await body()).replace(/\s+/g,' ').slice(0,90));

// 戻るジェスチャー（Android の端スワイプ相当）
await page.goBack();
await page.waitForTimeout(800);
check('戻るで今日に帰る', (await body()).includes('軸') || (await body()).includes('体重'));

// 設定（歯車）
await page.click('header button[aria-label="設定"]');
await page.waitForTimeout(1800);
const settings = await body();
check('歯車で設定が開く', settings.includes('週に通う回数'), settings.replace(/\s+/g,' ').slice(0,80));
check('休憩の長さが設定にある', settings.includes('休憩の長さ'));
check('トークンを消す手段がある', settings.includes('トークンを消す'));

// 設定からの戻る
await page.goBack();
await page.waitForTimeout(800);
check('設定から戻るでアプリが閉じない', (await body()).includes('体重'), (await body()).replace(/\s+/g,' ').slice(0,60));

// 記録 → タイマーが出る
const sets = await page.locator('button.set').count();
if (sets > 0) {
  await page.locator('button.set').first().click();
  await page.waitForTimeout(400);
  await page.fill('dialog input[inputmode=decimal]', '100');
  await page.fill('dialog input[inputmode=numeric] >> nth=0', '8');
  await page.click('dialog >> text=記録する');
  await page.waitForTimeout(1200);
  check('記録でタイマーバーが現れる', (await body()).includes('休憩中'));
}

await page.screenshot({ path: '/tmp/nav-today.png' });
await page.click('header button[aria-label="設定"]');
await page.waitForTimeout(1500);
await page.screenshot({ path: '/tmp/nav-settings.png' });

console.log('\nエラー:', errs.length ? errs.join('\n') : '(なし)');
const failed = results.filter((r) => !r.ok);
console.log(`\n${results.length - failed.length}/${results.length} 通過`);
await browser.close();
process.exit(failed.length ? 1 : 0);
