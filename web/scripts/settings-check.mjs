// 設定の保存を実機で確かめる。
//
// **週に通う回数が一番影響が大きい。**保存のあとに画面が自分で取り直せないと
// 空白になる。実際そこにバグがあった（描画時の program を掴んだ関数が、
// setProgram(null) の直後でも古い値を見て抜けていた）。単体テストでは踏めない。
//
// 使い方は scripts/ui-check.mjs と同じ。ポートは APP= と API= で渡す。
const { chromium } = await import(process.env.PLAYWRIGHT ?? 'playwright-core');
const APP = process.env.APP ?? 'http://localhost:4173';
const API = process.env.API ?? 'http://127.0.0.1:8080';
const TOKEN = 'dev-token-0123456789abcdef0123456789ab';
const auth = { Authorization: `Bearer ${TOKEN}` };
const program = () => fetch(`${API}/api/program`, { headers: auth }).then((r) => r.json());

const before = await program();
console.log('保存前: per_week =', before.per_week, '/ declared =', before.declared_exercises.length, '件');

const browser = await chromium.launch({ executablePath: process.env.CHROME ?? '/usr/bin/chromium', args: ['--no-sandbox'] });
const page = await (await browser.newContext({ viewport: { width: 390, height: 844 } })).newPage();
const errs = [];
page.on('pageerror', (e) => errs.push(e.message));
page.on('console', (m) => { if (m.type() === 'error') errs.push(m.text()); });

// ログインはフラグメントで済ませる。/auth/* を通すとプロバイダの画面が
// 挟まり、自動では抜けられない。#token= はサーバーがコールバックで戻して
// くる形そのものなので、取り込みの配線もここで一緒に検査できる。
await page.goto(`${APP}/#token=${TOKEN}`);
await page.waitForTimeout(2500);
await page.click('header button[aria-label="設定"]');
await page.waitForTimeout(1800);

// 設定の節は畳んである（縦 8.2画面分あったため）。開いてから触る。
await page.click('text=週に通う回数');
await page.waitForTimeout(500);

// 週に通う回数を変える（週目標も置き直るので、一番影響が大きい）
const want = before.per_week === 3 ? 4 : 3;
await page.selectOption('select', String(want));
await page.waitForTimeout(2500);

const after = await program();
console.log('保存後: per_week =', after.per_week, '（期待', want, '）');
console.log('他が壊れていないか: declared =', after.declared_exercises.length, '件 / selected =', after.selected_exercises.length, '件');
console.log('週目標の区分数:', Object.keys(after.weekly_target).length, '（利用者には出さない。サーバーが頻度から置き直す）');

// 元に戻す
await page.selectOption('select', String(before.per_week));
await page.waitForTimeout(2000);
const restored = await program();
console.log('戻した後: per_week =', restored.per_week);

console.log('エラー:', errs.length ? errs.join('\n') : '(なし)');
const ok = after.per_week === want && after.declared_exercises.length === before.declared_exercises.length && restored.per_week === before.per_week;
console.log(ok ? '\n✓ 設定の保存は壊れていない' : '\n✗ 壊れている');
await browser.close();
process.exit(ok ? 0 : 1);
