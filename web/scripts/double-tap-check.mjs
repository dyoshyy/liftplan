// 記録を素早く連打しても1回分しか記録されないことを実機で確かめる。
//
// 単体テストでは踏めないところを見る。二重に記録される原因は、記録の手順が
// 非同期（待ち行列への書き込みを待つ）で、待っている間もシートが開いたままで
// 「記録する」が押せること。React の状態も <dialog> も絡むので、DOM を
// 立てないと再現できない。
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
const clearToday = async () => {
  for (const x of await todaysSets()) {
    await fetch(`${API}/api/set-logs/${encodeURIComponent(x.id)}`, { method: 'DELETE', headers: auth });
  }
};
await clearToday();

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

const open = async (slot, w, r) => {
  await page.locator('button.set').nth(slot).click();
  await page.waitForTimeout(400);
  await page.fill('dialog input[inputmode=decimal]', String(w));
  await page.fill('dialog input[inputmode=numeric] >> nth=0', String(r));
};
const settle = () => page.waitForTimeout(2500);

// 1. 同じ tick に2回押す（最悪の連打）。
await open(0, 100, 5);
await page.evaluate(() => {
  const b = [...document.querySelectorAll('dialog button')].find((x) => x.textContent.includes('記録する'));
  b.click();
  b.click();
});
await settle();
let sets = await todaysSets();
check('同時に2回押しても、サーバーに届くのは1セット', sets.length === 1, `${sets.length}セット`);
const doneSlots = await page.locator('button.set-done').count();
check('画面の記録済みの枠も1つ', doneSlots === 1, `${doneSlots}枠`);

// 2. 少し間を置いて2回押す（人の連打）。
await clearToday();
await page.reload();
await page.waitForTimeout(2500);
await open(0, 100, 5);
await page.evaluate(async () => {
  const b = [...document.querySelectorAll('dialog button')].find((x) => x.textContent.includes('記録する'));
  b.click();
  await new Promise((r) => setTimeout(r, 15));
  b.click();
});
await settle();
sets = await todaysSets();
check('15ms 空けて2回押しても、サーバーに届くのは1セット', sets.length === 1, `${sets.length}セット`);

// 2b. 遅い端末で、人が連打する間隔（40ms）で2回押す。
//
// スマホは待ち行列への書き込み（IndexedDB）と描画が遅く、シートが閉じる前に
// 2回目が押せる。CPU を遅くして、その状況を作る。
const cdp = await page.context().newCDPSession(page);
await clearToday();
await page.reload();
await page.waitForTimeout(2500);
await open(0, 100, 5);
await cdp.send('Emulation.setCPUThrottlingRate', { rate: 20 });
await page.evaluate(async () => {
  const b = [...document.querySelectorAll('dialog button')].find((x) => x.textContent.includes('記録する'));
  b.click();
  await new Promise((r) => setTimeout(r, 40));
  if (b.isConnected) b.click();
});
await settle();
await cdp.send('Emulation.setCPUThrottlingRate', { rate: 1 });
await page.waitForTimeout(1500);
sets = await todaysSets();
check(
  '遅い端末で40ms空けて2回押しても、サーバーに届くのは1セット',
  sets.length === 1,
  `${sets.length}セット`,
);

// 3. 取り消しの連打。
await clearToday();
await page.reload();
await page.waitForTimeout(2500);
await open(0, 100, 5);
await page.click('dialog >> text=記録する');
await page.waitForTimeout(900);
await page.locator('button.set').nth(0).click();
await page.waitForTimeout(400);
await page.evaluate(() => {
  const b = [...document.querySelectorAll('dialog button')].find((x) =>
    x.textContent.includes('この記録を取り消す'),
  );
  b.click();
  b.click();
});
await settle();
sets = await todaysSets();
check('取り消しを連打しても、記録は消えている', sets.length === 0, `${sets.length}セット`);

// 4. 通常の記録は連打しなくても1回で通る（ガードが記録を止めていないこと）。
await clearToday();
await page.reload();
await page.waitForTimeout(2500);
for (const [slot, w, r] of [
  [0, 100, 5],
  [1, 100, 5],
  [2, 97.5, 6],
]) {
  await open(slot, w, r);
  await page.click('dialog >> text=記録する');
  await page.waitForTimeout(900);
}
await settle();
sets = await todaysSets();
check(
  '続けて3セット記録すると3セット届く（ガードが次の記録を止めない）',
  sets.length === 3,
  `${sets.length}セット`,
);

// ---- 楽観的更新：押した瞬間に画面が進み、保存は裏で回る ----
//
// 以下は、サーバーの応答を遅らせた状態を page.route で作る。保存の完了を
// 待たないので、遅れている間に起きることを見る。

const slowPage = async (delayMs, { failIdb = false } = {}) => {
  await clearToday();
  const ctx = await browser.newContext({ viewport: { width: 390, height: 844 }, deviceScaleFactor: 2 });
  if (failIdb) {
    // 端末への書き込みを、フラグが立っているあいだだけ失敗させる
    // （容量超過・プライベートモードの再現）。
    await ctx.addInitScript(() => {
      const add = IDBObjectStore.prototype.add;
      IDBObjectStore.prototype.add = function (...a) {
        if (window.__failIdb) throw new DOMException('容量が足りない', 'QuotaExceededError');
        return add.apply(this, a);
      };
    });
  }
  const p = await ctx.newPage();
  p.on('pageerror', (e) => errs.push('pageerror: ' + e.message));
  p.on('console', (m) => {
    if (m.type() === 'error') errs.push(m.text());
  });
  p.posts = 0;
  p.on('request', (r) => {
    if (r.method() === 'POST' && r.url().includes('/api/set-logs')) p.posts++;
  });
  await p.goto(`${APP}/?t=${Date.now()}#token=${TOKEN}`);
  await p.waitForTimeout(2500);
  if (delayMs) {
    await p.route('**/api/set-logs', async (route) => {
      if (route.request().method() === 'POST') await new Promise((r) => setTimeout(r, delayMs));
      await route.continue();
    });
  }
  return p;
};
const fillOn = async (p, slot, w, r) => {
  await p.locator('button.set').nth(slot).click();
  await p.waitForTimeout(400);
  await p.fill('dialog input[inputmode=decimal]', String(w));
  await p.fill('dialog input[inputmode=numeric] >> nth=0', String(r));
};
const closed = (p) =>
  p.waitForFunction(() => !document.querySelector('dialog[open]'), null, { timeout: 15000 });

// closes は閉じたかどうかを真偽で返す（例外にしない）。短く待つ。
const closes = (p, ms = 4000) =>
  p
    .waitForFunction(() => !document.querySelector('dialog[open]'), null, { timeout: ms })
    .then(
      () => true,
      () => false,
    );

// 5. 応答が遅いまま同じ tick に連打しても、送信は1回・記録は1セット。
//    操作のIDを固定してあるので、仮に2回通っても同じ記録に収まる。送信が
//    1回なのは、ラッチが副作用ごと2回目を止めているから。
{
  const p = await slowPage(2000);
  await fillOn(p, 0, 100, 5);
  await p.evaluate(() => {
    const b = [...document.querySelectorAll('dialog button')].find((x) => x.textContent.includes('記録する'));
    b.click();
    b.click();
  });
  await closed(p);
  check('応答が遅くても、画面の枠は1つだけ記録済み', (await p.locator('button.set-done').count()) === 1);
  // 送信は1件ずつ順に行う。2回通っていたら 2 秒 × 2 回かかるので、それを
  // 待ってから数える（短いと、2件目がまだ届いておらず見逃す）。
  await p.waitForTimeout(6000);
  check('応答が遅いまま連打しても、送信は1回', p.posts === 1, `${p.posts}回`);
  check('応答が遅いまま連打しても、サーバーに届くのは1セット', (await todaysSets()).length === 1);
  await p.context().close();
}

// 6. 保存を待つあいだに、続けて別のセットを記録しても、どちらも届く。
//    保存が終わるまで操作を止める門だと、2セット目が黙って捨てられる
//    （シートが閉じないまま残る）。例外ではなく ✗ で言うため、閉じたかどうかを
//    真偽で受ける。
{
  const p = await slowPage(2000);
  await fillOn(p, 0, 100, 5);
  await p.click('dialog >> text=記録する');
  check('1セット目を記録すると、シートが閉じる', await closes(p));
  await fillOn(p, 1, 102.5, 4);
  await p.click('dialog >> text=記録する');
  check('保存を待つあいだの2セット目も、押した直後にシートが閉じる', await closes(p));
  check(
    '保存を待つあいだに続けて記録すると、2枠とも記録済みで出る',
    (await p.locator('button.set-done').count()) === 2,
  );
  await p.waitForTimeout(4500);
  const sets = await todaysSets();
  check('続けて記録した2セットが、どちらもサーバーに届く', sets.length === 2, `${sets.length}セット`);
  await p.context().close();
}

// 7. 端末に保存できなかったら、画面を保存されている状態へ戻して知らせる。
//    戻さないと、画面には記録済みなのに何も保存されていない食い違いが残る。
{
  const p = await slowPage(0, { failIdb: true });
  await p.evaluate(() => {
    window.__failIdb = true;
  });
  await fillOn(p, 0, 100, 5);
  await p.click('dialog >> text=記録する');
  await closed(p);
  await p.waitForTimeout(1200);
  check('保存できなかった記録は、枠が元の「記録」に戻る', (await p.locator('button.set-done').count()) === 0);
  const text = await p.innerText('body');
  check('「保存できなかった記録」を知らせる', text.includes('保存できなかった記録'));
  check('何を記録しようとしたか（重量×レップ）が分かる', text.includes('100kg × 5'));
  check('サーバーには何も届いていない', (await todaysSets()).length === 0);

  // 直ったあとは、また記録できる（失敗が尾を引かない）。
  await p.evaluate(() => {
    window.__failIdb = false;
  });
  await fillOn(p, 0, 100, 5);
  await p.click('dialog >> text=記録する');
  await closed(p);
  await p.waitForTimeout(1500);
  check('保存できるようになったら、また記録できる', (await todaysSets()).length === 1);
  await p.context().close();
}

check('コンソールにエラーが出ない', errs.length === 0, errs.join(' | '));
await clearToday();
const failed = results.filter((r) => !r.ok);
console.log(`\n${results.length - failed.length}/${results.length} 通過`);
await browser.close();
process.exit(failed.length ? 1 : 0);
