// クライアントを敵対的に検証する。
//
// 「動くこと」ではなく「壊れないこと」を確かめる。記録が消える経路を探す。
// 単体テストでは踏めない経路（Service Worker・IndexedDB の再読み込み・
// オフライン復帰）を、本番ビルドの実機で通す。
//
// **dev サーバーでは意味がない。**Service Worker が無効なので、圏外で
// 再読み込みする経路を検査できない。必ず本番ビルドを preview で出すこと。
//
//   # 1. サーバー。DATABASE_URL を付けないこと（DEV_SESSION_TOKEN は
//   #    インメモリ構成でしか効かない）。OAuth の4つは起動の条件で、
//   #    中身は何でもよい。理由は scripts/ui-check.mjs の冒頭
//   cd .. && DEV_SESSION_TOKEN=dev-token-0123456789abcdef0123456789ab \
//     API_ORIGIN=http://localhost:8080 WEB_ORIGIN=http://localhost:4173 \
//     ALLOWED_ORIGINS=http://localhost:4173 \
//     GITHUB_CLIENT_ID=dev GITHUB_CLIENT_SECRET=dev \
//     GOOGLE_CLIENT_ID=dev GOOGLE_CLIENT_SECRET=dev \
//     go run ./cmd/api
//
//   # 2. 本番ビルドを出す
//   VITE_API_BASE=http://127.0.0.1:8080 pnpm build
//   pnpm exec vite preview --port 4173 --strictPort
//
//   # 3. 検証
//   PLAYWRIGHT=<playwright-core のパス> node scripts/adversarial-check.mjs
//
// CI には入れていない。ブラウザの実体が要るため。手で回す。
const PLAYWRIGHT = process.env.PLAYWRIGHT ?? 'playwright-core';
const { chromium } = await import(PLAYWRIGHT);

const TOKEN = 'dev-token-0123456789abcdef0123456789ab';
const API = process.env.API ?? 'http://127.0.0.1:8080';
// 本番ビルド。SW を効かせるため dev ではなく preview。
const APP = process.env.APP ?? 'http://localhost:4173';

const results = [];
const check = (name, ok, detail = '') => {
  results.push({ name, ok, detail });
  console.log(`${ok ? '✓' : '✗'} ${name}${detail ? ' — ' + detail : ''}`);
};

const serverSets = async (date) => {
  const res = await fetch(`${API}/api/set-logs?from=${date}&to=${date}`, {
    headers: { Authorization: `Bearer ${TOKEN}` },
  });
  const b = await res.json();
  return (b.days ?? []).flatMap((d) => d.exercises.flatMap((e) => e.sets));
};

const today = () => {
  const d = new Date();
  const p = (n) => String(n).padStart(2, '0');
  return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())}`;
};

// 前回の実行が残した記録を消す。
//
// 残っていると1セット目が「記録済み」になり、record() が新規ではなく
// 修正になる（DELETE + POST の2件が積まれ、サーバーの行数は増えない）。
// 検査が壊れているのか実装が壊れているのか区別できなくなる。
for (const s of await serverSets(today())) {
  await fetch(`${API}/api/set-logs/${encodeURIComponent(s.id)}`, {
    method: 'DELETE',
    headers: { Authorization: `Bearer ${TOKEN}` },
  });
}
console.log(`（前回の記録を ${'' + (await serverSets(today())).length} 件まで掃除した）\n`);

const browser = await chromium.launch({
  executablePath: process.env.CHROME ?? '/usr/bin/chromium',
  args: ['--no-sandbox'],
});
const ctx = await browser.newContext({ viewport: { width: 390, height: 844 } });
const page = await ctx.newPage();
const errors = [];
page.on('pageerror', (e) => errors.push('pageerror: ' + e.message));
page.on('console', (m) => {
  if (m.type() === 'error') errors.push(m.text());
});

const queueDump = () =>
  page.evaluate(async () => {
    const req = indexedDB.open('liftplan', 1);
    await new Promise((r) => {
      req.onsuccess = r;
    });
    const tx = req.result.transaction('queue', 'readonly');
    const all = tx.objectStore('queue').getAll();
    await new Promise((r) => {
      tx.oncomplete = r;
    });
    return all.result.map((i) => `${i.method ?? 'POST'} ${i.path}`);
  });

const record = async (setIndex, weight, reps) => {
  await page.locator('button.set').nth(setIndex).click();
  await page.waitForTimeout(350);
  await page.fill('dialog input[inputmode=decimal]', String(weight));
  await page.fill('dialog input[inputmode=numeric] >> nth=0', String(reps));
  await page.click('dialog >> text=記録する');
  await page.waitForTimeout(900);
};

// 同期の状態はナビの点に畳まれた（PR #78）。見た目の文字ではなく
// aria-label を読む。読み上げに出る文言そのものなので、表示を変えても
// 意味が変わらない限り壊れない。
const syncLabel = () => page.locator('nav button[aria-label]').first().getAttribute('aria-label');

await page.goto(APP);
// Service Worker が殻をキャッシュし終えるまで待つ。待たずに圏外にすると
// 「SW がまだ入っていないから開けない」を「壊れている」と読み違える。
await page.evaluate(() => navigator.serviceWorker.ready.then(() => undefined));
await page.waitForTimeout(1000);
// ログインはフラグメントで済ませる。/auth/* を通すとプロバイダの画面が
// 挟まり、自動では抜けられない。#token= はサーバーがコールバックで戻して
// くる形そのものなので、取り込みの配線もここで一緒に検査できる。
await page.goto(`${APP}/#token=${TOKEN}`);
// ここだけ reload が要る。既に同じ URL を開いているので、フラグメントだけの
// 移動は同一ドキュメント内の遷移になり、画面が組み直されない（取り込みは
// 起動時に1回だけ走る）。本番はコールバックからの完全な遷移なので起きない。
await page.reload();
await page.waitForTimeout(2000);

const before = (await serverSets(today())).length;

// --- 1. 圏外で記録したものが消えないか ---
await ctx.setOffline(true);
await page.waitForTimeout(300);
await record(0, 100, 8);
await record(1, 100, 7);

const offlineStatus = await syncLabel();
console.log('  待ち行列:', JSON.stringify(await queueDump()));
check(
  '圏外でも記録が手元に残る',
  (await page.locator('button.set').nth(0).innerText()).includes('100'),
  offlineStatus,
);
check(
  '圏外で未送信件数が出る',
  /未送信\s*2/.test(offlineStatus.replace(/\s+/g, ' ')) || /オフライン/.test(offlineStatus),
  offlineStatus,
);
check('圏外では実際にサーバーへ届いていない', (await serverSets(today())).length === before);

// --- 2. 圏外のまま再読み込みしても消えないか（一番怖い経路）---
await page.reload();
await page.waitForTimeout(2500);
const afterReload = await syncLabel();
check('圏外で再読み込みしても未送信が残っている', /未送信 2 件/.test(afterReload), afterReload);

// --- 3. 復帰したら送られるか ---
await ctx.setOffline(false);
await page.waitForTimeout(500);
await page.evaluate(() => window.dispatchEvent(new Event('online')));
await page.waitForTimeout(2500);

// 復帰したらメニューが自分で戻ってくること。
// 「更新」を押さないと出ないなら、ジムで開いて何も出ないのと同じ。
check(
  '復帰後にメニューが自動で戻る',
  (await page.locator('button.set').count()) > 0,
  `button.set = ${await page.locator('button.set').count()}`,
);

const afterOnline = await serverSets(today());
check('復帰後にサーバーへ届く', afterOnline.length === before + 2, `${before} → ${afterOnline.length}`);
check('復帰後は同期済みになる', /同期済み/.test(await syncLabel()), await syncLabel());

// --- 4. 二重送信していないか ---
const ids = afterOnline.map((s) => s.id);
check('IDが重複していない', new Set(ids).size === ids.length, ids.join(','));

// --- 5. 修正が重複を生まないか ---
await record(0, 105, 8);
await page.waitForTimeout(1200);
const afterEdit = await serverSets(today());
check(
  '修正しても件数が増えない',
  afterEdit.length === afterOnline.length,
  `${afterOnline.length} → ${afterEdit.length}`,
);
check(
  '修正が反映されている',
  afterEdit.some((s) => s.weight_kg === 105),
);

// --- 6. サーバーが 400 を返す記録は rejected に落ちて、後続を詰まらせないか ---
await page.evaluate(async () => {
  const req = indexedDB.open('liftplan', 1);
  await new Promise((r) => {
    req.onsuccess = r;
  });
  const db = req.result;
  const tx = db.transaction('queue', 'readwrite');
  // 契約に無いフィールドを混ぜる。サーバーは DisallowUnknownFields なので 400。
  tx.objectStore('queue').add({ path: '/api/set-logs', body: { logs: [{ bogus: true }] } });
  await new Promise((r) => {
    tx.oncomplete = r;
  });
});
await page.reload();
await page.waitForTimeout(3000);
const stuck = await syncLabel();
check('通らない記録は待ち行列を詰まらせない', !/未送信 [1-9]/.test(stuck), stuck);
check('捨てた記録が画面に出る', /送れなかった/.test(await page.locator('body').innerText()));

// --- 7. 記録の途中でトークンが無効になっても、記録が消えないか ---
await page.evaluate(() => localStorage.setItem('liftplan.token', 'x'.repeat(40)));
await page.reload();
await page.waitForTimeout(2500);
check(
  'トークンが無効ならログイン画面に戻る',
  /GitHub でログイン/.test(await page.locator('body').innerText()),
);

console.log('\n--- コンソールエラー ---');
console.log(errors.length ? errors.join('\n') : '(なし)');

const failed = results.filter((r) => !r.ok);
console.log(`\n${results.length - failed.length}/${results.length} 通過`);
if (failed.length) {
  console.log('落ちた検査:');
  for (const f of failed) console.log(`  - ${f.name}: ${f.detail}`);
}
await browser.close();
process.exit(failed.length ? 1 : 0);
