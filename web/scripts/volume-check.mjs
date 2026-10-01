// 休憩終了の音量を実機で確かめる。
//
// 単体テストでは踏めないところを見る。設定画面の入力 → フック → 実際の音
// （Web Audio のゲイン）まで、値が本当に届くか。音の計画（volume.ts）は単体
// テストが守っているので、ここは配線を見る。
//
// ゲインは、AudioParam の linearRampToValueAtTime に渡されたピークと、
// 作られた発振器の数を横取りして読む。耳では確かめられないので。
//
// 使い方は scripts/ui-check.mjs と同じ。ポートは APP= で渡す。
// 休憩の終わりまで待つので、1分ほどかかる。
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
const clearToday = async () => {
  const sets = await fetch(`${API}/api/set-logs?from=${day()}&to=${day()}`, { headers: auth })
    .then((r) => r.json())
    .then((b) => (b.days ?? []).flatMap((d) => d.exercises.flatMap((e) => e.sets)));
  for (const s of sets)
    await fetch(`${API}/api/set-logs/${encodeURIComponent(s.id)}`, { method: 'DELETE', headers: auth });
};
await clearToday();

const browser = await chromium.launch({
  executablePath: process.env.CHROME ?? '/usr/bin/chromium',
  // ヘッドレスでも音の再生を始められるようにする（操作なしでも止めない）。
  args: ['--no-sandbox', '--autoplay-policy=no-user-gesture-required'],
});
const ctx = await browser.newContext({ viewport: { width: 390, height: 844 }, deviceScaleFactor: 2 });
const page = await ctx.newPage();
const errs = [];
page.on('pageerror', (e) => errs.push('pageerror: ' + e.message));
page.on('console', (m) => {
  if (m.type() === 'error') errs.push(m.text());
});

// 音を横取りする。ピークの大きさと、発振器の数。
// 休憩は最短（15秒）にして、終わるまでの待ちを短くする。
await ctx.addInitScript(() => {
  window.__peaks = [];
  window.__oscillators = 0;
  const ramp = AudioParam.prototype.linearRampToValueAtTime;
  AudioParam.prototype.linearRampToValueAtTime = function (value, t) {
    window.__peaks.push(value);
    return ramp.call(this, value, t);
  };
  const create = AudioContext.prototype.createOscillator;
  AudioContext.prototype.createOscillator = function () {
    window.__oscillators++;
    return create.call(this);
  };
  if (!localStorage.getItem('liftplan.rest.duration')) localStorage.setItem('liftplan.rest.duration', '15');
});

// 開き直す。page.reload() は使わない。再読み込みしても history.state は
// 残るが、アプリの画面状態は「今日」から始まる。設定で再読み込みすると、
// 履歴は設定のまま・画面は今日、になり、あとで「今日」へ戻ろうとして
// history.back() が設定に戻る（既存の挙動。この検査の対象ではない）。
const reopen = async () => {
  // 同じURLへの goto は再読み込み扱いで、履歴が置き換わり状態が残る。別のURLで開く。
  await page.goto(`${APP}/?t=${Date.now()}`);
  await page.waitForTimeout(2500);
};
const peaks = () => page.evaluate(() => [...window.__peaks]);
const oscillators = () => page.evaluate(() => window.__oscillators);
const resetCounters = () =>
  page.evaluate(() => {
    window.__peaks = [];
    window.__oscillators = 0;
  });
const near = (a, b) => Math.abs(a - b) < 1e-9;

await page.goto(`${APP}/#token=${TOKEN}`);
await page.waitForTimeout(2500);

const openSection = async () => {
  await page.click('header button[aria-label="設定"]');
  await page.waitForTimeout(1500);
  await page.click('text=休憩終了の音量');
  await page.waitForTimeout(400);
};
const volumeInput = () =>
  page.locator('div:has(> label:has-text("休憩が終わったときの合図")) input[type=number]');
const summary = () => page.locator('button[aria-controls]', { hasText: '休憩終了の音量' }).innerText();

await openSection();
check('既定の音量は50', (await volumeInput().inputValue()) === '50', await volumeInput().inputValue());

// 試し聴き: 既定は、音量を足す前の音（ピーク0.25）と同じ。
await page.click('text=試しに鳴らす');
await page.waitForTimeout(500);
let p = await peaks();
check('試し聴きで3音鳴る', p.length === 3, `${p.length}音`);
check('既定のピークは0.25（これまでと同じ）', p.length === 3 && p.every((v) => near(v, 0.25)), p.join(','));
check('試し聴きは休憩タイマーを始めない', !(await page.innerText('body')).includes('休憩中'));

// ＋で上げる。5回押して100。
for (let i = 0; i < 6; i++) await page.click('button[aria-label="10 増やす"]');
await page.waitForTimeout(300);
check('＋は100で止まる', (await volumeInput().inputValue()) === '100', await volumeInput().inputValue());
await resetCounters();
await page.click('text=試しに鳴らす');
await page.waitForTimeout(500);
p = await peaks();
check('100ならピークは1', p.length === 3 && p.every((v) => near(v, 1)), p.join(','));

// 入力欄に直接打つ。
await volumeInput().fill('30');
await page.waitForTimeout(300);
await resetCounters();
await page.click('text=試しに鳴らす');
await page.waitForTimeout(500);
p = await peaks();
check('30ならピークは0.09', p.length === 3 && p.every((v) => near(v, 0.09)), p.join(','));

// 範囲の外は100に収まる。
await volumeInput().fill('250');
await page.waitForTimeout(300);
check(
  '250と打っても100に収まる',
  (await volumeInput().inputValue()) === '100',
  await volumeInput().inputValue(),
);

// 0 は鳴らさない。
await volumeInput().fill('0');
await page.waitForTimeout(300);
await resetCounters();
await page.click('text=試しに鳴らす');
await page.waitForTimeout(500);
check('0なら発振器を作らない（鳴らさない）', (await oscillators()) === 0, `${await oscillators()}個`);
await page.click('button[aria-controls]:has-text("休憩終了の音量")');
await page.waitForTimeout(300);
const folded = await summary();
check('0のとき、畳んだ節は「音なし」', folded.includes('音なし'), folded.replace(/\n/g, ' '));

// 保存: 開き直しても残る。
await reopen();
await openSection();
check('開き直しても音量が残る', (await volumeInput().inputValue()) === '0', await volumeInput().inputValue());

// 休憩が終わったときに、設定した音量で鳴る（配線の本体）。
// 記録すると休憩が始まる（15秒）。終わるまで待つ。
const finishWith = async (vol) => {
  await volumeInput().fill(String(vol));
  await page.waitForTimeout(300);
  await page.click('nav >> text=今日');
  await page.waitForTimeout(1500);
  await resetCounters();
  if ((await page.locator('button.set').count()) === 0) {
    console.log(
      '（今日の画面に記録の枠が無い）',
      (await page.innerText('body')).slice(0, 300).replace(/\n/g, ' | '),
    );
  }
  await page.locator('button.set').nth(0).click({ timeout: 5000 });
  await page.waitForTimeout(400);
  await page.fill('dialog input[inputmode=decimal]', '60');
  await page.fill('dialog input[inputmode=numeric] >> nth=0', '5');
  await page.click('dialog >> text=記録する');
  await page.waitForTimeout(900);
  await page.waitForFunction(() => document.body.innerText.includes('休憩中'), null, { timeout: 5000 });
  await resetCounters(); // 記録時に鳴った音（無い）を数えない。終わりの合図だけを見る。
  await page
    .waitForFunction(() => window.__oscillators > 0 || !document.body.innerText.includes('休憩中'), null, {
      timeout: 25000,
    })
    .catch(() => {});
  await page.waitForTimeout(1500);
};

await finishWith(100);
p = await peaks();
check(
  '休憩が終わると、100の音量（ピーク1）で3音鳴る',
  p.length === 3 && p.every((v) => near(v, 1)),
  p.join(','),
);

await clearToday();
await reopen();
await page.click('header button[aria-label="設定"]');
await page.waitForTimeout(1500);
await page.click('text=休憩終了の音量');
await page.waitForTimeout(400);
await finishWith(0);
check('0なら、休憩が終わっても鳴らさない', (await oscillators()) === 0, `${await oscillators()}個`);

check('コンソールにエラーが出ない', errs.length === 0, errs.join(' | '));
await clearToday();
const failed = results.filter((r) => !r.ok);
console.log(`\n${results.length - failed.length}/${results.length} 通過`);
await browser.close();
process.exit(failed.length ? 1 : 0);
