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
const check = (n, ok, d = '') => {
  results.push({ n, ok, d });
  console.log(`${ok ? '✓' : '✗'} ${n}${d ? ' — ' + d : ''}`);
};

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

const body = () => page.innerText('body');
const navCount = () => page.locator('nav').count();

check('ナビが1本出る', (await navCount()) === 1);
check('待機中はタイマーバーが出ない', !(await body()).includes('休憩中'));
check('体重が1行になっている', (await page.locator('label[for=bw]').count()) === 1);
check('設定は今日の画面に無い', !(await body()).includes('週に通う回数'));

// 履歴タブ
await page.click('nav >> text=履歴');
await page.waitForTimeout(1800);
check('履歴が開く', /\d{4}年\d{1,2}月/.test(await body()), (await body()).replace(/\s+/g, ' ').slice(0, 90));

// 戻るジェスチャー（Android の端スワイプ相当）
await page.goBack();
await page.waitForTimeout(800);
check('戻るで今日に帰る', (await body()).includes('軸') || (await body()).includes('体重'));

// 設定（歯車）
await page.click('header button[aria-label="設定"]');
await page.waitForTimeout(1800);
const settings = await body();
check('歯車で設定が開く', settings.includes('週に通う回数'), settings.replace(/\s+/g, ' ').slice(0, 80));
check('休憩の長さが設定にある', settings.includes('休憩の長さ'));
// 設定の節は畳んである（縦 8.2画面分あったため）。開いてから確かめる。
await page.click('text=アカウント');
await page.waitForTimeout(500);
check('ログアウトする手段がある', (await body()).includes('ログアウト'));

// 設定からの戻る
await page.goBack();
await page.waitForTimeout(800);
check(
  '設定から戻るでアプリが閉じない',
  (await body()).includes('体重'),
  (await body()).replace(/\s+/g, ' ').slice(0, 60),
);

// 種目ページは設定の子。戻るで設定に帰る（#228。以前は今日に帰っていた）。
const today = async () => (await body()).includes('体重');
const isSettings = async () => (await body()).includes('週に通う回数');
const isExercises = async () => (await body()).includes('種目を追加');
const openExercises = async () => {
  await page.click('header button[aria-label="設定"]');
  await page.waitForTimeout(1500);
  await page.getByText(/使う\d+・伸ばす/).click();
  await page.waitForTimeout(500);
  await page.getByRole('button', { name: '種目を管理する' }).click();
  await page.waitForTimeout(1000);
};
await openExercises();
check('設定から種目ページが開く', await isExercises());
await page.goBack();
await page.waitForTimeout(800);
check('種目ページから戻るで設定に帰る', await isSettings(), (await body()).replace(/\s+/g, ' ').slice(0, 60));
await page.goBack();
await page.waitForTimeout(800);
check('設定からもう一度戻ると今日', await today());

// 「← 設定」も戻るジェスチャーと同じ段へ帰る。積み直すと、戻るで種目ページに戻ってしまう。
await openExercises();
await page.getByRole('button', { name: '← 設定' }).click();
await page.waitForTimeout(800);
check('「← 設定」で設定に帰る', await isSettings());
await page.goBack();
await page.waitForTimeout(800);
check('「← 設定」のあとの戻るは今日', await today(), (await body()).replace(/\s+/g, ' ').slice(0, 60));

// 種目ページから下のナビで他の画面へ移ったあとも、戻るは今日へ帰る（設定に出ない）。
await openExercises();
await page.click('nav >> text=履歴');
await page.waitForTimeout(1500);
check('種目ページから履歴が開く', /\d{4}年\d{1,2}月/.test(await body()));
await page.goBack();
await page.waitForTimeout(800);
check('そこから戻ると今日', await today(), (await body()).replace(/\s+/g, ' ').slice(0, 60));

await openExercises();
await page.click('nav >> text=今日');
await page.waitForTimeout(1200);
check('種目ページから今日へ', await today());

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
