// 補助の「埋まっている」を実機で確かめる。
//
// 単体テストでは踏めないところを見る。押すとその場でカードが代わりに
// 差し替わること、もう一度押すと rejected を付けて次の候補を頼むこと、
// 連打しても1回しか取りに行かないこと、記録したカードからボタンが消えること、
// 開き直しても代わりが残ること。判断（何を送るか・どのカードを出すか・
// 何を残すか）は features/today/substitute.ts の単体テストが守っているので、
// ここは配線を見る。
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

// 前回の実行が残した記録を消す。残っていると、記録済みのカードに「埋まっている」が出ない。
const API = process.env.API ?? 'http://127.0.0.1:8080';
const day = () => {
  const d = new Date();
  const p = (n) => String(n).padStart(2, '0');
  return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())}`;
};
const auth = { Authorization: `Bearer ${TOKEN}` };
const todaysSets = () =>
  fetch(`${API}/api/set-logs?from=${day()}&to=${day()}`, { headers: auth })
    .then((r) => r.json())
    .then((b) =>
      (b.days ?? []).flatMap((d) =>
        d.exercises.flatMap((e) => e.sets.map((s) => ({ ...s, exercise_id: e.exercise_id }))),
      ),
    );
for (const x of await todaysSets()) {
  await fetch(`${API}/api/set-logs/${encodeURIComponent(x.id)}`, { method: 'DELETE', headers: auth });
}

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

await page.goto(`${APP}/#token=${TOKEN}`);
await page.waitForTimeout(1500);
// 前回の実行が残した差し替えを消してから読み直す。
await page.evaluate(() => localStorage.removeItem('liftplan.substitutes'));
await page.reload();
await page.waitForTimeout(2500);

const body = () => page.innerText('body');
const cardNames = () => page.locator('span.font-bold.text-\\[17px\\]').allInnerTexts();
const cardOf = (name) =>
  page.locator('div.grid.gap-3', { has: page.locator(`span.font-bold:text-is("${name}")`) });
const blockedOf = (name) => cardOf(name).getByRole('button', { name: '埋まっている' });

const requests = [];
page.on('request', (r) => {
  if (r.url().includes('/api/sessions/substitute')) requests.push(r.url());
});

const before = await cardNames();
const nBlocked = await page.getByRole('button', { name: '埋まっている' }).count();
check('補助のカードに「埋まっている」が出る', nBlocked > 0, `${nBlocked}件`);
const firstPlanned = before[0];
check('軸のカードには出ない', (await blockedOf(firstPlanned).count()) === 0, firstPlanned);

// 連打：再描画の前に2回押す。取得は1回だけのはず。
await page.evaluate(() => {
  const b = [...document.querySelectorAll('button')].find((x) => x.textContent === '埋まっている');
  if (!b) return;
  b.click();
  b.click();
});
await page.waitForTimeout(1500);
check('連打しても取得は1回', requests.length === 1, requests.join(' | '));
check('表示中の計画の日付で頼む', (requests[0] ?? '').includes(`date=${day()}`), requests[0] ?? '');

const after = await cardNames();
const replaced = before.find((n) => !after.includes(n));
const sub = after.find((n) => !before.includes(n));
let current;
if (sub) {
  check('カードが代わりに差し替わる', !!replaced, `${replaced} → ${sub}`);
  check('何の代わりかが出る', (await body()).includes(`${replaced} の代わり`));
  check('並びの位置は変わらない', after.indexOf(sub) === before.indexOf(replaced), after.join(','));

  // もう一度押す：元の replace のまま、出した代わりを rejected に積む。
  await blockedOf(sub).click();
  await page.waitForTimeout(1500);
  const second = requests[1] ?? '';
  const rid = new URL(requests[0]).searchParams.get('replace');
  check(
    '2回目は元の replace と rejected で頼む',
    new URL(second).searchParams.get('replace') === rid &&
      new URL(second).searchParams.getAll('rejected').length === 1,
    second,
  );
  const third = await cardNames();
  const next = third.find((n) => !before.includes(n));
  check(
    '2回目は別の候補か「代わりなし」',
    (next && next !== sub) || (await body()).includes('代わりなし。飛ばしてよい'),
    third.join(','),
  );
  current = next ?? replaced;
} else {
  check('代わりが無ければ「代わりなし」と出る', (await body()).includes('代わりなし。飛ばしてよい'));
  current = replaced ?? after[1];
}

// 記録したカードからはボタンが消える。
if ((await blockedOf(current).count()) === 1) {
  await cardOf(current).locator('button.set').nth(0).click();
  await page.waitForTimeout(400);
  await page.fill('dialog input[inputmode=decimal]', '20');
  await page.fill('dialog input[inputmode=numeric] >> nth=0', '10');
  await page.click('dialog >> text=記録する');
  await page.waitForTimeout(900);
  check('記録したカードには「埋まっている」が出ない', (await blockedOf(current).count()) === 0, current);
}

// 開き直しても差し替えは残る（localStorage）。
const beforeReload = await cardNames();
await page.reload();
await page.waitForTimeout(2500);
const reloaded = await cardNames();
check(
  '開き直しても同じカードが並ぶ',
  JSON.stringify(reloaded) === JSON.stringify(beforeReload),
  reloaded.join(','),
);
check('開き直しても元の補助は出ない', !replaced || !sub || !reloaded.includes(replaced), reloaded.join(','));

console.log('\nエラー:', errs.length ? errs.join('\n') : '(なし)');
check('コンソールにエラーが出ない', errs.length === 0, errs.join(' | '));

// 後片付け。差し替えと記録を次の実行に持ち越さない。
await page.evaluate(() => localStorage.removeItem('liftplan.substitutes'));
for (const x of await todaysSets()) {
  await fetch(`${API}/api/set-logs/${encodeURIComponent(x.id)}`, { method: 'DELETE', headers: auth });
}
const failed = results.filter((r) => !r.ok);
console.log(`\n${results.length - failed.length}/${results.length} 通過`);
await browser.close();
process.exit(failed.length ? 1 : 0);
