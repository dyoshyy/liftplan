// グループ（仲間）の画面を実機で確かめる。
//
// 単体テストは判断（格子・自分を先頭・リンクの形）しか見ていない。
// ここで見るのは配線：タブから開けるか、作ったグループの格子に自分が出るか、
// ● を押すと種目が出るか、リンクの共有・作り直し・抜けるが画面内の確認を
// 経て効くか。
//
// インメモリの API は開発用の1人しか入れないので、2人目の参加は API の
// テスト（internal/presentation/httpapi/groups_test.go）が見ている。
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

const auth = { Authorization: `Bearer ${TOKEN}` };
const day = () => {
  const d = new Date();
  const p = (n) => String(n).padStart(2, '0');
  return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())}`;
};

// 前回の実行が残したグループから抜ける。残っていると最初に作る画面が出ない。
const before = await fetch(`${API}/api/groups`, { headers: auth }).then((r) => r.json());
for (const g of before.groups ?? []) {
  await fetch(`${API}/api/groups/${g.id}/members/me`, { method: 'DELETE', headers: auth });
}
// 今日の記録を1件入れておく。格子の今日のマスに ● が出るはず。
await fetch(`${API}/api/set-logs`, {
  method: 'POST',
  headers: { ...auth, 'Content-Type': 'application/json' },
  body: JSON.stringify({
    logs: [
      { id: `groups-check-${Date.now()}`, date: day(), exercise_id: 'bench', weight_kg: 60, reps: 5, rir: 2 },
    ],
  }),
});

const browser = await chromium.launch({
  executablePath: process.env.CHROME ?? '/usr/bin/chromium',
  args: ['--no-sandbox'],
});
const context = await browser.newContext({ viewport: { width: 390, height: 844 }, deviceScaleFactor: 2 });
await context.grantPermissions(['clipboard-read', 'clipboard-write'], { origin: APP });
const page = await context.newPage();
const errs = [];
page.on('pageerror', (e) => errs.push('pageerror: ' + e.message));
page.on('console', (m) => {
  if (m.type() === 'error') errs.push(m.text());
});
// 共有シートは headless では開けない。無いことにしてクリップボードの経路を通す。
await page.addInitScript(() => {
  Object.defineProperty(navigator, 'share', { value: undefined, configurable: true });
});

await page.goto(`${APP}/#token=${TOKEN}`);
await page.waitForTimeout(2000);
const body = () => page.innerText('body');

await page.getByRole('button', { name: '仲間', exact: true }).click();
await page.waitForTimeout(800);
check('グループが無ければ作る画面が出る', (await body()).includes('グループを作る'));

const create = page.getByRole('button', { name: '作る', exact: true });
check('名前が空なら作れない', await create.isDisabled());
await page.getByLabel('グループの名前').fill('ジム仲間');
await page.getByLabel('このグループでの自分の名前').fill('よし');
await create.click();
await page.waitForTimeout(1000);

const text = await body();
check('作ったグループの名前が出る', text.includes('ジム仲間'));
check('格子に自分の行が出る', text.includes('よし') && text.includes('(自分)'));

const dot = page.getByRole('button', { name: /よし .* の種目/ });
check('今日のマスに ● が1つ出る', (await dot.count()) === 1);
await dot.first().click();
await page.waitForTimeout(300);
check('● を押すとその日の種目が出る', (await body()).includes('ベンチプレス'));
check('重量は出ない', !(await body()).includes('60kg') && !(await body()).includes('60 kg'));

await page.getByRole('button', { name: '招待リンクを共有' }).click();
await page.waitForTimeout(300);
const first = await page.evaluate(() => navigator.clipboard.readText());
check('共有するとリンクがコピーされる', (await body()).includes('リンクをコピーしました'));
check('リンクは画面のオリジンに invite が付いた形', first.startsWith(`${APP}/?invite=`), first);

await page.getByRole('button', { name: 'リンクを作り直す' }).click();
check('作り直す前に画面内で確認する', (await body()).includes('今のリンクは使えなくなります'));
await page.getByRole('button', { name: '作り直す', exact: true }).click();
await page.waitForTimeout(600);
await page.getByRole('button', { name: '招待リンクを共有' }).click();
await page.waitForTimeout(300);
const second = await page.evaluate(() => navigator.clipboard.readText());
check('作り直すとリンクが替わる', second !== first && second.startsWith(`${APP}/?invite=`));
const old = new URL(first).searchParams.get('invite');
const oldStatus = await fetch(`${API}/api/invites/${encodeURIComponent(old)}`, { headers: auth }).then(
  (r) => r.status,
);
check('作り直す前のリンクは使えない', oldStatus === 404, String(oldStatus));

await page.getByRole('button', { name: 'グループを抜ける' }).click();
check('抜ける前に画面内で確認する', (await body()).includes('お互いの記録が見えなくなります'));
await page.getByRole('button', { name: '抜ける', exact: true }).click();
await page.waitForTimeout(800);
check('最後の1人が抜けると作る画面に戻る', (await body()).includes('グループを作る'));
const after = await fetch(`${API}/api/groups`, { headers: auth }).then((r) => r.json());
check('抜けたグループは一覧から消える', after.groups.length === 0);

await page.goBack();
await page.waitForTimeout(500);
check('戻るで今日の画面に戻る', !(await body()).includes('グループを作る'));

check('コンソールにエラーが無い', errs.length === 0, errs.join(' | '));

await browser.close();
const failed = results.filter((r) => !r.ok);
console.log(`\n${results.length - failed.length}/${results.length} 通過`);
process.exit(failed.length ? 1 : 0);
