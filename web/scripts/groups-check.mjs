// グループの画面を実機で確かめる。
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

await page.getByRole('button', { name: 'グループ', exact: true }).click();
await page.waitForTimeout(800);
check('グループが無ければ作る画面が出る', (await body()).includes('グループを作る'));

const create = page.getByRole('button', { name: '作る', exact: true });
check('名前が空なら作れない', await create.isDisabled());
await page.getByLabel('グループの名前').fill('ジム仲間');
await page.getByLabel('このグループでの自分の名前').fill('よし');
await create.click();
await page.waitForTimeout(1000);

let text = await body();
check('作るとそのグループの詳細が開く', text.includes('ジム仲間') && text.includes('今週の記録'));
check('格子に自分の行が出る', text.includes('よし') && text.includes('(自分)'));

const dot = page.getByRole('button', { name: /よし .* の記録へ/ });
check('今日のマスに ● が1つ出る', (await dot.count()) === 1);
check('押さなくても、その日の種目が記録に出る', text.includes('ベンチプレス'));
check('重量は出ない', !text.includes('60kg') && !text.includes('60 kg'));
await dot.first().click();
await page.waitForTimeout(300);
check('詳細に「別のグループを作る」は無い', !(await body()).includes('別のグループを作る'));

await page.locator('main').getByRole('button', { name: 'グループ', exact: true }).click();
await page.waitForTimeout(800);
text = await body();
check(
  '「グループ」で一覧に戻る',
  text.includes('ジム仲間') && text.includes('1人') && !text.includes('今週の記録'),
);
check(
  '一覧に「グループを作る」がある',
  (await page.getByRole('button', { name: 'グループを作る' }).count()) === 1,
);
check('一覧にグループへの操作は無い', !text.includes('招待リンクを共有'));

await page.getByRole('button', { name: /ジム仲間/ }).click();
await page.waitForTimeout(800);
await page.goBack();
await page.waitForTimeout(800);
check('詳細で戻ると一覧に戻る', (await body()).includes('グループを作る'));
await page.getByRole('button', { name: /ジム仲間/ }).click();
await page.waitForTimeout(800);

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
check('最後の1人が抜けると一覧（空なので作る画面）に戻る', (await body()).includes('グループを作る'));
const after = await fetch(`${API}/api/groups`, { headers: auth }).then((r) => r.json());
check('抜けたグループは一覧から消える', after.groups.length === 0);

await page.goBack();
await page.waitForTimeout(500);
check('戻るで今日の画面に戻る', !(await body()).includes('グループを作る'));

// --- 招待リンクから入る ---
//
// 認証の戻り先は固定なので、?invite= はログインの往復で落ちる。開いた時点で
// 端末に覚え、URL からは消し、ログインのあとで聞く。

const fresh = await browser.newContext({ viewport: { width: 390, height: 844 } });
const p2 = await fresh.newPage();
p2.on('pageerror', (e) => errs.push('pageerror: ' + e.message));
await p2.goto(`${APP}/?invite=no-such-token`);
await p2.waitForTimeout(1200);
check(
  'ログイン前でも招待を受け取ったと出る',
  (await p2.innerText('body')).includes('ログインすると参加できます'),
);
check('招待のトークンは URL から消える', !p2.url().includes('invite='), p2.url());

await p2.goto(`${APP}/#token=${TOKEN}`);
await p2.reload();
await p2.waitForTimeout(2000);
check(
  'ログインのあと、使えないリンクだと分かる',
  (await p2.innerText('body')).includes('このリンクは使えません'),
);
await p2.getByRole('button', { name: '閉じる' }).click();
await p2.waitForTimeout(300);
check(
  '閉じると招待は消える',
  (await p2.evaluate(() => localStorage.getItem('liftplan.groups.invite'))) === null,
);

const made = await fetch(`${API}/api/groups`, {
  method: 'POST',
  headers: { ...auth, 'Content-Type': 'application/json' },
  body: JSON.stringify({ name: '会社', display_name: '吉田' }),
}).then((r) => r.json());

await p2.goto(`${APP}/?invite=${encodeURIComponent(made.invite_token)}`);
await p2.waitForTimeout(1500);
check(
  '自分のグループのリンクなら参加済みと出る',
  (await p2.innerText('body')).includes('「会社」には参加しています'),
);
await p2.getByRole('button', { name: '見る', exact: true }).click();
await p2.waitForTimeout(1200);
check('見るとそのグループの詳細が開く', (await p2.innerText('body')).includes('吉田'));

// 2人目は作れないので、リンクの中身だけ「未参加」に差し替える。参加の送信は
// 本物に送る（既にメンバーなので冪等に同じグループの ID が返る）。
await p2.route(`${API}/api/invites/${encodeURIComponent(made.invite_token)}`, (route) =>
  route.fulfill({
    status: 200,
    contentType: 'application/json',
    headers: { 'Access-Control-Allow-Origin': APP },
    body: JSON.stringify({ group_id: made.id, name: '会社', member_count: 1, already_member: false }),
  }),
);
await p2.getByRole('button', { name: '今日', exact: true }).click();
await p2.goto(`${APP}/?invite=${encodeURIComponent(made.invite_token)}`);
await p2.waitForTimeout(1500);
check('未参加なら参加するかを聞く', (await p2.innerText('body')).includes('「会社」（1人）に参加しますか'));
const joinButton = p2.getByRole('button', { name: '参加する', exact: true });
check('名前を入れるまで参加できない', await joinButton.isDisabled());
await p2.getByLabel('このグループでの自分の名前').fill('たろう');
await joinButton.click();
await p2.waitForTimeout(1500);
const joinedText = await p2.innerText('body');
check('参加するとそのグループの詳細が開く', joinedText.includes('今週') && joinedText.includes('吉田'));
check(
  '参加したら招待は消える',
  (await p2.evaluate(() => localStorage.getItem('liftplan.groups.invite'))) === null,
);
await fetch(`${API}/api/groups/${made.id}/members/me`, { method: 'DELETE', headers: auth });

check('コンソールにエラーが無い', errs.length === 0, errs.join(' | '));

await browser.close();
const failed = results.filter((r) => !r.ok);
console.log(`\n${results.length - failed.length}/${results.length} 通過`);
process.exit(failed.length ? 1 : 0);
