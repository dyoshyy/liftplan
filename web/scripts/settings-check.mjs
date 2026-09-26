// 設定の保存を実機で確かめる。
//
// **週に通う回数が一番影響が大きい。**以前は保存のたびに設定を取り直して
// いて、そのせいで画面が空白になったり（描画時の program を掴んだ関数が
// 古い値を見て抜けていた）、畳んだ節で動かしていた未保存のチェックが
// 消えたりした。どちらも単体テストでは踏めない。
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

// 「使う種目」で1つチェックを外し、保存せずに置いておく。回数を変えたあとも
// 外れたままかを見る。初期状態は全種目が選ばれているので、入れるのではなく
// 外す。伸ばしたい種目は外せない（disabled）ので、押せるものから選ぶ。
// 節は畳んであるので開いてから触る。
const pickHeader = page.getByRole('button', { name: /^使う種目/ });
await pickHeader.click();
await page.waitForTimeout(500);
const pickBody = page.locator(`[id="${await pickHeader.getAttribute('aria-controls')}"]`);
const toUnpick = pickBody.locator('button:not([disabled])').filter({ hasText: '✓' }).first();
const unpickedName = (await toUnpick.textContent())?.replace('✓', '').trim();
await toUnpick.click();
const chip = pickBody.locator('button', { hasText: unpickedName });
const SAVE_PICK = 'button:has-text("使う種目を保存する")';
const pendingBefore = await page.locator(SAVE_PICK).isVisible();

// 週に通う回数を変える。「通い方」は最初から開いている。
const want = before.per_week === 3 ? 4 : 3;
// aria-label で指す。'select' だと先頭の1つを掴むので、選択が増えると
// 別の設定を黙って触る。
const FREQ = 'select[aria-label="週に通う回数"]';
await page.selectOption(FREQ, String(want));
await page.waitForTimeout(2500);

const after = await program();
console.log('保存後: per_week =', after.per_week, '（期待', want, '）');
const pendingAfter = await page.locator(SAVE_PICK).isVisible();
const stillUnchecked = !(await chip.textContent())?.includes('✓');
console.log('未保存で外したチェック（', unpickedName, '）: 変更前', pendingBefore, '/ 変更後', pendingAfter, stillUnchecked, '（期待 true / true true）');
console.log('他が壊れていないか: declared =', after.declared_exercises.length, '件 / selected =', after.selected_exercises.length, '件');

// 元に戻す
await page.selectOption(FREQ, String(before.per_week));
await page.waitForTimeout(2000);
const restored = await program();
console.log('戻した後: per_week =', restored.per_week);

// 1回の量。片方の選択を変えたとき、もう片方が送られずに 0 で断られる
// 配線ミス（400）もここで出る。
const EX = 'select[aria-label="1回の種目数"]';
const wantEx = restored.exercises_per_session === 4 ? 5 : 4;
await page.selectOption(EX, String(wantEx));
await page.waitForTimeout(2500);
const vol = await program();
console.log('1回の量: ', vol.exercises_per_session, '種目 ×', vol.sets_per_exercise, 'セット（期待', wantEx, '種目 ×', restored.sets_per_exercise, 'セット）');
await page.selectOption(EX, String(restored.exercises_per_session));
await page.waitForTimeout(2000);
const volRestored = await program();

// 外しておいたチェックを入れ直して元に戻す（保存はしていない）。
await chip.click();
const pendingCleared = !(await page.locator(SAVE_PICK).isVisible());

console.log('エラー:', errs.length ? errs.join('\n') : '(なし)');
const ok = after.per_week === want && after.declared_exercises.length === before.declared_exercises.length && restored.per_week === before.per_week
  && vol.exercises_per_session === wantEx && vol.sets_per_exercise === restored.sets_per_exercise
  && volRestored.exercises_per_session === restored.exercises_per_session
  && pendingBefore && pendingAfter && stillUnchecked && pendingCleared
  && after.selected_exercises.length === before.selected_exercises.length;
console.log(ok ? '\n✓ 設定の保存は壊れていない' : '\n✗ 壊れている');
await browser.close();
process.exit(ok ? 0 : 1);
