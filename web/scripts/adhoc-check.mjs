// 「種目を選んで記録」を実機で確かめる。
//
// 単体テストでは踏めないところを見る。選択シートの開閉（<dialog>）、選んだ種目の
// カードが出ること、記録のたびに次の空き枠が出ること、記録したあとに
// 「今日やったもの」へ二重に出ないこと。判断（何を選択肢に出すか）は
// features/exercises/pickable.ts と features/today/adhoc.ts（plannedIds）の
// 単体テストが守っているので、ここは配線を見る。
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

// 前回の実行が残した記録を消す。残っていると「今日やったもの」の数が合わない。
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
await page.waitForTimeout(2500);

const body = () => page.innerText('body');
const entry = page.getByRole('button', { name: '種目を選んで記録' });
const dialogButtons = () => page.locator('dialog[open] button');
const cardNames = () => page.locator('span.font-bold.text-\\[17px\\]').allInnerTexts();

check('入口のボタンが出る', (await entry.count()) === 1);

// 予定に出ている種目の名前。選択肢に出てはいけない。
const plannedNames = await cardNames();
check('前提: 予定のカードがある', plannedNames.length > 0, plannedNames.join(','));

await entry.click();
await page.waitForTimeout(400);
check('選択シートが開く', (await page.locator('dialog[open]').count()) === 1);

const options = await dialogButtons().allInnerTexts();
check('選択肢に種目が並ぶ', options.length > 5, `${options.length}件`);
check(
  '予定に出ている種目は選択肢に無い',
  plannedNames.every((n) => !options.includes(n)),
  `予定=${plannedNames.join(',')}`,
);

// 絞り込み。
const first = options.find((o) => o !== '閉じる');
const keyword = first.slice(0, 2);
await page.fill('dialog input[type=search]', keyword);
await page.waitForTimeout(300);
const filtered = (await dialogButtons().allInnerTexts()).filter((o) => o !== '閉じる');
check(
  '名前で絞れる',
  filtered.length > 0 && filtered.every((o) => o.toLowerCase().includes(keyword.toLowerCase())),
  `「${keyword}」→ ${filtered.join(',')}`,
);

await page.fill('dialog input[type=search]', 'ありえない名前zzz');
await page.waitForTimeout(300);
check('一致が無ければ案内が出る', (await body()).includes('該当する種目がありません'));
await page.fill('dialog input[type=search]', '');
await page.waitForTimeout(300);

// 選ぶ。
await page.locator('dialog[open] button', { hasText: first }).first().click();
await page.waitForTimeout(500);
check('選ぶとシートが閉じる', (await page.locator('dialog[open]').count()) === 0);
check('「選んだ種目」の見出しが出る', (await body()).includes('選んだ種目'));
const afterPick = await cardNames();
check('選んだ種目のカードが出る', afterPick.includes(first), afterPick.join(','));
check('目標は出さない（自分で選んだ種目）', (await body()).includes('自分で選んだ種目・好きなだけ'));

// カードの枠は空き1つだけ。
const slotsOf = (name) =>
  page
    .locator('div.grid.gap-3', { has: page.locator(`span.font-bold:text-is("${name}")`) })
    .locator('button.set');
check('記録前の枠は1つ', (await slotsOf(first).count()) === 1);

// 1セット目を記録。
const record = async (n, w, r) => {
  await slotsOf(first).nth(n).click();
  await page.waitForTimeout(400);
  await page.fill('dialog input[inputmode=decimal]', String(w));
  await page.fill('dialog input[inputmode=numeric] >> nth=0', String(r));
  await page.click('dialog >> text=記録する');
  await page.waitForTimeout(900);
};
await record(0, 40, 10);
check('記録すると次の空き枠が出る', (await slotsOf(first).count()) === 2);
check('1セット目が記録済みで出る', (await slotsOf(first).nth(0).innerText()).includes('40×10'));

await record(1, 42.5, 8);
check('2セット目でも次の空き枠が出る', (await slotsOf(first).count()) === 3);

// 二重に出ない。
const heading = (await body()).split('今日やったもの').length - 1;
check('「今日やったもの」に同じ種目が二重に出ない', heading === 0, `見出し${heading}件`);
check('カードは1枚だけ', (await cardNames()).filter((n) => n === first).length === 1);

// サーバーに届いている。
const saved = (await todaysSets()).filter((s) => s.weight_kg === 40 || s.weight_kg === 42.5);
check(
  'サーバーに2セット届く',
  saved.length === 2,
  JSON.stringify(saved.map((s) => [s.exercise_id, s.weight_kg, s.reps])),
);

// 同じ種目を選び直しても増えない。
await entry.click();
await page.waitForTimeout(400);
check(
  '選び済みの種目は選択肢に残る（追加のために選び直せる）',
  (await dialogButtons().allInnerTexts()).includes(first),
);
await page.locator('dialog[open] button', { hasText: first }).first().click();
await page.waitForTimeout(500);
check('選び直してもカードは増えない', (await cardNames()).filter((n) => n === first).length === 1);

// 開き直すと、選んだ状態は消え、やった事実は「今日やったもの」に残る。
await page.reload();
await page.waitForTimeout(2500);
check('開き直すと「選んだ種目」の見出しは消える', !(await body()).includes('選んだ種目\n'));
check(
  '開き直しても記録は「今日やったもの」に残る',
  (await body()).includes('今日やったもの') && (await cardNames()).includes(first),
);

await page.screenshot({ path: '/tmp/adhoc-check.png' });
console.log('\nエラー:', errs.length ? errs.join('\n') : '(なし)');
check('コンソールにエラーが出ない', errs.length === 0, errs.join(' | '));

// 後片付け。
for (const x of await todaysSets()) {
  await fetch(`${API}/api/set-logs/${encodeURIComponent(x.id)}`, { method: 'DELETE', headers: auth });
}
const failed = results.filter((r) => !r.ok);
console.log(`\n${results.length - failed.length}/${results.length} 通過`);
await browser.close();
process.exit(failed.length ? 1 : 0);
