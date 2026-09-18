// 休憩タイマーと記録シートの初期値を実機で確かめる。
//
// 単体テストでは踏めないところを見る。タイマーは時間と localStorage と
// 再読み込みをまたぐので、純粋な関数の検査だけでは「画面で本当に動くか」が
// 分からない。初期値も、シートを開いたときに何が入るかは DOM でしか見えない。
//
// **本番ビルドを preview で出して使う。**使い方:
//
//   # サーバー
//   cd .. && AUTH_TOKEN=dev-token-0123456789abcdef0123456789ab \
//     ALLOWED_ORIGINS=http://localhost:4173 PORT=8080 go run ./cmd/api
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
const check = (n, ok, d = '') => { results.push({ n, ok, d }); console.log(`${ok ? '✓' : '✗'} ${n}${d ? ' — ' + d : ''}`); };

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

const browser = await chromium.launch({ executablePath: process.env.CHROME ?? '/usr/bin/chromium', args: ['--no-sandbox'] });
const page = await (await browser.newContext({ viewport: { width: 390, height: 844 }, deviceScaleFactor: 2 })).newPage();
const errs = [];
page.on('pageerror', (e) => errs.push('pageerror: ' + e.message));
page.on('console', (m) => { if (m.type() === 'error') errs.push(m.text()); });

await page.goto(APP);
await page.fill('input[type=password]', TOKEN);
await page.click('text=保存する');
await page.waitForTimeout(2500);

const timerText = () => page.locator('.fixed.z-40').first().innerText();
check('タイマーバーが出る', /\d:\d\d/.test(await timerText()), (await timerText()).replace(/\s+/g, ' '));
check('既定は3分', (await timerText()).includes('3:00'));

// 1セット目を記録する
const record = async (i, w, r) => {
  await page.locator('button.set').nth(i).click();
  await page.waitForTimeout(400);
  await page.fill('dialog input[inputmode=decimal]', String(w));
  await page.fill('dialog input[inputmode=numeric] >> nth=0', String(r));
  await page.click('dialog >> text=記録する');
  await page.waitForTimeout(900);
};
await record(0, 102.5, 6);

const afterRecord = await timerText();
check('記録で自動スタートする', /休憩中/.test(afterRecord), afterRecord.replace(/\s+/g, ' '));

// 秒は切り上げるので、記録直後はまだ 3:00 のまま。1秒以上待って減ることを見る。
await page.waitForTimeout(1500);
const ticked = await timerText();
check('カウントダウンしている', !ticked.includes('3:00'), `${afterRecord.match(/\d:\d\d/)[0]} → ${ticked.match(/\d:\d\d/)[0]}`);

// 2セット目の初期値
await page.locator('button.set').nth(1).click();
await page.waitForTimeout(500);
const w2 = await page.inputValue('dialog input[inputmode=decimal]');
const r2 = await page.inputValue('dialog input[inputmode=numeric] >> nth=0');
check('2セット目の重量が1セット目に揃う', w2 === '102.5', `重量=${w2}`);
check('2セット目のレップが1セット目に揃う', r2 === '6', `レップ=${r2}`);
await page.click('dialog >> text=閉じる');
await page.waitForTimeout(400);

// 一時停止とリセット
await page.click('text=一時停止');
await page.waitForTimeout(1500);
const paused1 = await timerText();
await page.waitForTimeout(2000);
const paused2 = await timerText();
check('止めているあいだは減らない', paused1.match(/\d:\d\d/)[0] === paused2.match(/\d:\d\d/)[0], `${paused1.match(/\d:\d\d/)[0]} → ${paused2.match(/\d:\d\d/)[0]}`);

await page.click('text=リセット');
await page.waitForTimeout(500);
check('リセットで既定に戻る', (await timerText()).includes('3:00'));

// 長さを変えて永続化
await page.locator('.fixed.z-40 button').first().click();
await page.waitForTimeout(400);
const minus = page.locator('.fixed.z-40 button[aria-label*="減らす"]');
await minus.click(); await minus.click(); await minus.click(); await minus.click();
await page.waitForTimeout(400);
const shortened = await timerText();
check('長さを変えられる', shortened.includes('2:00'), shortened.replace(/\s+/g, ' '));

await page.reload();
await page.waitForTimeout(2500);
check('長さが再読み込みをまたいで残る', (await timerText()).includes('2:00'), (await timerText()).replace(/\s+/g, ' '));

await page.screenshot({ path: '/tmp/timer-ui.png' });
console.log('\nエラー:', errs.length ? errs.join('\n') : '(なし)');
const failed = results.filter((r) => !r.ok);
console.log(`\n${results.length - failed.length}/${results.length} 通過`);
await browser.close();
process.exit(failed.length ? 1 : 0);
