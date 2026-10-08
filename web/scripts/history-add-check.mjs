// 履歴から記録を足すのを実機で確かめる。
//
// 単体テストは「何を積むか」（planHistoryAdd）と「どの日まで足せるか」
// （addableRange）しか見ていない。History に種目の一覧と使う種目が届くか、
// 3つの入口（記録を足す・種目を足す・セットを足す）が出るか、足す／直すの
// どちらのシートからどちらの手順が呼ばれるかは配線なので、ここで見る。
//
// 使い方は scripts/ui-check.mjs と同じ。ポートは APP= と API= で渡す。
// 前の月の10日を使い、始めと終わりにその日の記録を消す。
const { chromium } = await import(process.env.PLAYWRIGHT ?? 'playwright-core');
const APP = process.env.APP ?? 'http://localhost:4173';
const API = process.env.API ?? 'http://127.0.0.1:8080';
const TOKEN = 'dev-token-0123456789abcdef0123456789ab';
const auth = { Authorization: `Bearer ${TOKEN}` };
const results = [];
const check = (n, ok, d = '') => {
  results.push({ n, ok, d });
  console.log(`${ok ? '✓' : '✗'} ${n}${d ? ' — ' + d : ''}`);
};

const p2 = (n) => String(n).padStart(2, '0');
const iso = (d) => `${d.getFullYear()}-${p2(d.getMonth() + 1)}-${p2(d.getDate())}`;
const now = new Date();
const TODAY = iso(now);
const DATE = iso(new Date(now.getFullYear(), now.getMonth() - 1, 10));
const PREV_FIRST = iso(new Date(now.getFullYear(), now.getMonth() - 1, 1));
const PREV_LAST = iso(new Date(now.getFullYear(), now.getMonth(), 0));
const THIS_FIRST = iso(new Date(now.getFullYear(), now.getMonth(), 1));
const LABEL = `${now.getMonth() === 0 ? 12 : now.getMonth()}/10`;

const setsOn = (date) =>
  fetch(`${API}/api/set-logs?from=${date}&to=${date}`, { headers: auth })
    .then((r) => r.json())
    .then((b) =>
      (b.days ?? []).flatMap((d) =>
        d.exercises.flatMap((e) => e.sets.map((s) => ({ ...s, date: d.date, exercise_id: e.exercise_id }))),
      ),
    );
const clear = async (date) => {
  for (const x of await setsOn(date)) {
    await fetch(`${API}/api/set-logs/${encodeURIComponent(x.id)}`, { method: 'DELETE', headers: auth });
  }
};
const until = async (pred, ms = 8000) => {
  const end = Date.now() + ms;
  for (;;) {
    const v = await pred();
    if (v || Date.now() > end) return v;
    await new Promise((r) => setTimeout(r, 300));
  }
};

await clear(DATE);
const program = await fetch(`${API}/api/program`, { headers: auth }).then((r) => r.json());
const catalog = (await fetch(`${API}/api/exercises`, { headers: auth }).then((r) => r.json())).exercises;
const nameOf = new Map(catalog.map((e) => [e.id, e.name]));
const selectedNames = program.selected_exercises.map((id) => nameOf.get(id)).sort();
console.log(`前提: 使う種目 ${selectedNames.length} / 全種目 ${catalog.length}`);

const browser = await chromium.launch({
  executablePath: process.env.CHROME ?? '/usr/bin/chromium',
  args: ['--no-sandbox'],
});
const context = await browser.newContext({ viewport: { width: 390, height: 844 } });
const page = await context.newPage();
const errs = [];
page.on('pageerror', (e) => errs.push('pageerror: ' + e.message));
page.on('console', (m) => {
  if (m.type() === 'error') errs.push(m.text());
});

const body = () => page.innerText('body');
const dialog = page.locator('dialog[open]');
const pickerOptions = async () =>
  (await dialog.locator('section button').allInnerTexts()).map((t) => t.trim()).sort();
const openHistoryPrevMonth = async () => {
  await page.click('nav >> text=履歴');
  await page.waitForTimeout(800);
  await page.getByRole('button', { name: '前の月', exact: true }).click();
  await page.waitForTimeout(1500);
};

await page.goto(`${APP}/#token=${TOKEN}`);
await page.waitForTimeout(2500);
await page.click('nav >> text=履歴');
await page.waitForTimeout(800);

const dateInput = page.getByLabel('記録を足す日');
const addRecord = page.getByRole('button', { name: '記録を足す', exact: true });
check('今月: 記録を足すの入口が出る', (await addRecord.count()) === 1);
check(
  '今月: 足せる日は1日から今日まで',
  (await dateInput.getAttribute('min')) === THIS_FIRST && (await dateInput.getAttribute('max')) === TODAY,
  `${await dateInput.getAttribute('min')}〜${await dateInput.getAttribute('max')}`,
);

await page.getByRole('button', { name: '前の月', exact: true }).click();
await page.waitForTimeout(1500);
check(
  '前の月: 足せる日は1日から月末まで',
  (await dateInput.getAttribute('min')) === PREV_FIRST && (await dateInput.getAttribute('max')) === PREV_LAST,
  `${await dateInput.getAttribute('min')}〜${await dateInput.getAttribute('max')}`,
);
check('前提: 足す日に記録が無い', !(await body()).includes(LABEL));

// 記録を足す → 種目を選ぶ → 値 → 足す。
await dateInput.fill(DATE);
await addRecord.click();
await page.waitForTimeout(500);
check('記録を足すと種目を選ぶシートが開く', (await dialog.count()) === 1);
check('シートの見出しに足す日が出る', (await dialog.innerText()).includes(`${LABEL} (`));
const options = await pickerOptions();
check(
  '選択肢は使う種目（History に exercises と selected が届いている）',
  JSON.stringify(options) === JSON.stringify(selectedNames),
  `${options.length}件`,
);
const first = options[0];
await dialog.locator('section button', { hasText: first }).first().click();
await page.waitForTimeout(600);
const sheetText = await dialog.innerText();
check('選ぶと足すシートが開く', sheetText.includes(`${first} 1セット目を足す`), sheetText.split('\n')[0]);
check('足すシートに「消す」は出ない', !sheetText.includes('このセットを消す'));
const weight = dialog.locator('input[inputmode=decimal]');
const reps = dialog.locator('input[inputmode=numeric]').nth(0);
const rir = dialog.locator('input[inputmode=numeric]').nth(1);
check('新しい種目の入力は空', (await weight.inputValue()) === '' && (await reps.inputValue()) === '');
await weight.fill('52.5');
await reps.fill('7');
await rir.fill('2');
await dialog.getByRole('button', { name: '足す', exact: true }).click();
await page.waitForTimeout(600);
check('足すとシートが閉じる', (await dialog.count()) === 0);
check('足した日が一覧に出る', (await body()).includes(LABEL) && (await body()).includes(first));

const sent = await until(async () => {
  const s = await setsOn(DATE);
  return s.length === 1 ? s : null;
});
check(
  'サーバーに足す日の日付で届く',
  sent?.[0]?.date === DATE && sent?.[0]?.weight_kg === 52.5 && sent?.[0]?.reps === 7,
  JSON.stringify(sent),
);
check('今日の日付では届いていない', !(await setsOn(TODAY)).some((s) => s.weight_kg === 52.5 && s.reps === 7));

// セットを足す。入力は最後のセットの値で始まる。
await page
  .getByRole('button', { name: new RegExp(`^${first}`) })
  .first()
  .click();
await page.waitForTimeout(300);
await page.getByRole('button', { name: `${first}にセットを足す` }).click();
await page.waitForTimeout(500);
check(
  'セットを足すと2セット目の足すシートが開く',
  (await dialog.innerText()).includes(`${first} 2セット目を足す`),
);
check(
  '入力は最後のセットの値で始まる',
  (await weight.inputValue()) === '52.5' && (await reps.inputValue()) === '7',
);
await reps.fill('6');
await dialog.getByRole('button', { name: '足す', exact: true }).click();
await page.waitForTimeout(600);
const two = await until(async () => {
  const s = await setsOn(DATE);
  return s.length === 2 ? s : null;
});
check('2セット目が同じ日に届く', two?.every((s) => s.date === DATE) === true, JSON.stringify(two));

// 種目を足す。その日に記録がある種目は選択肢に無い。
await page.getByRole('button', { name: new RegExp(`^${LABEL} \\(.\\)に種目を足す$`) }).click();
await page.waitForTimeout(500);
const dayOptions = await pickerOptions();
check(
  '種目を足す: その日に記録がある種目は選択肢に無い',
  dayOptions.length === selectedNames.length - 1 && !dayOptions.includes(first),
  `${dayOptions.length}件`,
);
await dialog.getByRole('button', { name: '閉じる', exact: true }).click();
await page.waitForTimeout(400);

// 直すシートは直す手順を呼ぶ（足すと取り違えると、同じ値のセットが増える）。
await page.getByRole('button', { name: `${first} 1セット目を直す` }).click();
await page.waitForTimeout(500);
const editText = await dialog.innerText();
check(
  '直すシートには「を足す」が付かず「消す」が出る',
  !editText.includes('を足す') && editText.includes('このセットを消す'),
);
await weight.fill('55');
await dialog.getByRole('button', { name: '直す', exact: true }).click();
const edited = await until(async () => {
  const s = await setsOn(DATE);
  return s.length === 2 && s.some((x) => x.id === sent?.[0]?.id && x.weight_kg === 55) ? s : null;
});
check('直すと同じ ID のまま値が変わる（増えない）', edited !== null, JSON.stringify(await setsOn(DATE)));

// 開き直しても残る。
await page.reload();
await page.waitForTimeout(2500);
await openHistoryPrevMonth();
check('開き直しても足した日が残る', (await body()).includes(LABEL) && (await body()).includes(first));

// 読めない月には入口を出さない。
const failing = await context.newPage();
await failing.route(`**/api/set-logs?from=${PREV_FIRST}*`, (route) =>
  route.fulfill({ status: 500, body: '{}' }),
);
await failing.goto(`${APP}/#token=${TOKEN}`);
await failing.waitForTimeout(2500);
await failing.click('nav >> text=履歴');
await failing.waitForTimeout(800);
await failing.getByRole('button', { name: '前の月', exact: true }).click();
await failing.waitForTimeout(1500);
check(
  '読めなかった月には記録を足すを出さない',
  (await failing.innerText('body')).includes('もう一度読む') &&
    (await failing.getByRole('button', { name: '記録を足す', exact: true }).count()) === 0,
);

await page.screenshot({ path: '/tmp/history-add-check.png' });
console.log('\nエラー:', errs.length ? errs.join('\n') : '(なし)');
check('コンソールにエラーが出ない', errs.length === 0, errs.join(' | '));

await clear(DATE);
const failed = results.filter((r) => !r.ok);
console.log(`\n${results.length - failed.length}/${results.length} 通過`);
await browser.close();
process.exit(failed.length ? 1 : 0);
