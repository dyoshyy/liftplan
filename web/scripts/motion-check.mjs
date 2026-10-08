// 画面の動きが、操作を待たせず、描き直しで入り直さず、「動きを減らす」で
// 止まることを実機で確かめる。
//
// 単体テストでは見られない。CSS の animation は要素が足されたときと class が
// 付いたときにだけ走るので、守れているかは key の付け方と DOM の作り直しで
// 決まる。描いてみないと分からない。
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

const day = () => {
  const d = new Date();
  const p = (n) => String(n).padStart(2, '0');
  return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())}`;
};
const auth = { Authorization: `Bearer ${TOKEN}` };
const todaysSets = () =>
  fetch(`${API}/api/set-logs?from=${day()}&to=${day()}`, { headers: auth })
    .then((r) => r.json())
    .then((b) => (b.days ?? []).flatMap((d) => d.exercises.flatMap((e) => e.sets)));
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

const running = (sel) =>
  page.evaluate(
    (s) =>
      [...document.querySelectorAll(s)].flatMap((el) =>
        el
          .getAnimations({ subtree: true })
          .filter((a) => a.playState === 'running')
          .map((a) => a.animationName),
      ),
    sel,
  );
const animationName = (sel, pseudo = null) =>
  page.evaluate(
    ([s, p]) => {
      const el = document.querySelector(s);
      return el ? getComputedStyle(el, p).animationName : 'missing';
    },
    [sel, pseudo],
  );
const settled = () =>
  page.waitForFunction(() => document.getAnimations().length === 0, null, { timeout: 5000 });
// 自己ベストの演出は画面を覆う。次の操作の前に閉じる。数えてから押すまでの
// あいだに自分で閉じることがあるので、押せなくても待つだけにする。
const dismissPR = async () => {
  await page
    .locator('.pr-overlay')
    .click({ timeout: 1000 })
    .catch(() => {});
  await page.locator('.pr-overlay').waitFor({ state: 'detached' });
};
const load = async () => {
  await page.goto(`${APP}/?t=${Date.now()}#token=${TOKEN}`);
  await page.locator('button.set').first().waitFor();
  await settled();
};
const recordButton = () =>
  page.evaluate(() => {
    const b = [...document.querySelectorAll('dialog button')].find((x) => x.textContent.includes('記録する'));
    const r = b.getBoundingClientRect();
    const x = r.left + r.width / 2;
    const y = r.top + r.height / 2;
    const sheet = document
      .querySelector('dialog')
      .getAnimations()
      .find((a) => a.playState === 'running');
    return { x, y, hit: b.contains(document.elementFromPoint(x, y)), at: sheet?.currentTime ?? null };
  });

await load();

const delays = await page.evaluate(() =>
  [...document.querySelectorAll('main .animate-rise-in')].map((el) =>
    parseFloat(getComputedStyle(el).animationDelay),
  ),
);
check(
  'カードは上から順に遅れて出る',
  delays.length >= 3 && delays.every((d, i) => i === 0 || d > delays[i - 1]),
  `${delays.length}枚 ${delays.map((d) => d * 1000).join('/')}ms`,
);
// 今日のカードは8枚に届かないことが多い。頭打ちは12枚並べて確かめる。
const longList = await page.evaluate(() => {
  const box = document.createElement('div');
  box.innerHTML = '<div class="animate-rise-in"></div>'.repeat(12);
  document.querySelector('main').append(box);
  const d = [...box.children].map((el) => parseFloat(getComputedStyle(el).animationDelay) * 1000);
  box.remove();
  return d;
});
check(
  '長い一覧でも、遅れは 8 枚目で頭打ちになる',
  longList[7] > 0 && longList.slice(7).every((d) => d === longList[7]),
  longList.join('/') + 'ms',
);

// ---- シート ----
// 入力を埋めるあいだも動きが続くように、時間の進みを 1/20 にする。
const cdp = await page.context().newCDPSession(page);
await cdp.send('Animation.enable');
await cdp.send('Animation.setPlaybackRate', { playbackRate: 0.05 });
await page.locator('button.set').first().click();
check('シートを開くと、下から出る動きが付く', (await animationName('dialog[open]')) === 'sheet-up');
check('シートの背景も暗くなる動きが付く', (await animationName('dialog[open]', '::backdrop')) === 'fade-in');
await page.fill('dialog input[inputmode=decimal]', '60');
await page.fill('dialog input[inputmode=numeric] >> nth=0', '5');

// 動いている途中のボタンの位置を押す。押せないなら、記録は増えない。
const before = (await todaysSets()).length;
const target = await recordButton();
await page.mouse.click(target.x, target.y);
await cdp.send('Animation.setPlaybackRate', { playbackRate: 1 });
check(
  '動いている途中のシートでも「記録する」は押した場所にある',
  target.at !== null && target.hit,
  `動き始めから ${target.at === null ? '（止まっていた）' : Math.round(target.at) + 'ms'}`,
);

// ---- 記録したセット ----
await page.waitForFunction(() => document.querySelectorAll('button.set-done').length === 1);
const pop = await running('button.set-done');
check('記録したセットが弾む', pop.includes('set-pop'), pop.join(',') || 'なし');
await page.waitForTimeout(1500);
const after = (await todaysSets()).length;
check('動いている途中に押した記録が1件届く', after === before + 1, `${before} → ${after}`);

await dismissPR();
await settled();
await page.evaluate(() => {
  for (const el of document.querySelectorAll('main .animate-rise-in')) el.dataset.motionMark = '1';
});
await page.locator('button.set:not(.set-done)').first().click();
await page.locator('dialog >> text=記録する').click();
await page.waitForFunction(() => document.querySelectorAll('button.set-done').length === 2);
const reentered = await running('main .animate-rise-in');
const remounted = await page.evaluate(
  () => [...document.querySelectorAll('main .animate-rise-in')].filter((el) => !el.dataset.motionMark).length,
);
check(
  'セットを記録しても、ほかのカードは入り直さない',
  !reentered.includes('rise-in') && remounted === 0,
  `入り直し ${reentered.filter((n) => n === 'rise-in').length} / 作り直し ${remounted}`,
);
const pops = await page.evaluate(
  () =>
    [...document.querySelectorAll('button.set-done')].filter((el) =>
      el.getAnimations().some((a) => a.playState === 'running' && a.animationName === 'set-pop'),
    ).length,
);
check('弾むのは、いま記録した1枠だけ', pops === 1, `${pops}枠`);

await page.waitForTimeout(1500);
await dismissPR();
await load();
const done = await page.locator('button.set-done').count();
const popsOnLoad = await page.evaluate(() => document.querySelectorAll('.set-pop').length);
check(
  '開き直したとき、済んだ枠は弾まない',
  done === 2 && popsOnLoad === 0,
  `済み ${done}枠 / 弾む ${popsOnLoad}枠`,
);

// ---- 画面の切り替え ----
await page.locator('nav >> text=履歴').click();
const screen = await running('main > div:last-child');
check('画面を切り替えると、中身に入りの動きが付く', screen.includes('fade-in'), screen.join(',') || 'なし');
await settled();

// 履歴で直したときも、日のカードは入り直さない。
await page.evaluate(() => {
  for (const el of document.querySelectorAll('main .animate-rise-in')) el.dataset.motionMark = '1';
});
await page.locator('main .animate-rise-in button[aria-expanded="false"]').first().click();
const unfold = await running('main .animate-unfold');
check('種目の行を開くと、中身が下から出る', unfold.includes('rise-in'), unfold.join(',') || 'なし');
await page.locator('button[aria-label$="1セット目を直す"]').first().click();
await page.fill('dialog input[inputmode=numeric] >> nth=0', '6');
await page.locator('dialog').getByRole('button', { name: '直す', exact: true }).click();
await page.locator('dialog').waitFor({ state: 'detached' });
const historyReentered = await running('main .animate-rise-in');
const historyRemounted = await page.evaluate(
  () => [...document.querySelectorAll('main .animate-rise-in')].filter((el) => !el.dataset.motionMark).length,
);
check(
  '履歴でセットを直しても、日のカードは入り直さない',
  !historyReentered.includes('rise-in') && historyRemounted === 0,
  `入り直し ${historyReentered.filter((n) => n === 'rise-in').length} / 作り直し ${historyRemounted}`,
);

await page.locator('nav >> text=今日').click();
await page.locator('button.set').first().waitFor();
await settled();

// ---- 動きを減らす ----
await page.emulateMedia({ reducedMotion: 'reduce' });
await page.locator('button.set:not(.set-done)').first().click();
const sheetReduced = await animationName('dialog[open]');
const backdropReduced = await animationName('dialog[open]', '::backdrop');
check(
  '動きを減らす設定では、シートは動かない',
  sheetReduced === 'none' && backdropReduced === 'none',
  `${sheetReduced} / ${backdropReduced}`,
);
await page.locator('dialog >> text=記録する').click();
await page.waitForFunction(() => document.querySelectorAll('button.set-done').length === 3);
const popReduced = await page.evaluate(() => document.getAnimations().map((a) => a.animationName));
check(
  '動きを減らす設定では、記録しても何も動かない',
  popReduced.length === 0,
  popReduced.join(',') || 'なし',
);
await page.waitForTimeout(1500);
await dismissPR();
await page.locator('nav >> text=履歴').click();
const screenReduced = await animationName('main > div:last-child');
const anyReduced = await page.evaluate(() => document.getAnimations().length);
check(
  '動きを減らす設定では、画面を切り替えても動かない',
  screenReduced === 'none' && anyReduced === 0,
  `${screenReduced} / 動いている数 ${anyReduced}`,
);

await clearToday();
await browser.close();
check('ページのエラーが無い', errs.length === 0, errs.join(' | '));
const failed = results.filter((r) => !r.ok);
console.log(`\n${results.length - failed.length}/${results.length} passed`);
process.exit(failed.length ? 1 : 0);
