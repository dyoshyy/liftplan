import { execSync } from 'node:child_process';
// 更新バナーを実機で確かめる。
//
// **古い Service Worker から抜け出せるか**を見る。skipWaiting を呼ばない
// 設計なので、押せる場所が無いと、開いたままの端末は古い版を実行し続ける。
// 実際そうなって、super reload でしか復帰できなかったことがある。
//
// 踏んだ落とし穴が2つある。どちらも「実装が壊れている」と誤診しかけた。
//
// 1. **コメントを足しても minify で消え、sw.js が1バイトも変わらない。**
//    更新が検知されないのは当たり前で、実装は正しかった。出力が実際に
//    変わる変更（ここでは index.html の title）にする
// 2. **初回訪問のページは SW の制御下に無い。**新しい SW が waiting を
//    経ずに有効化され、押しても何も起きないように見える。一度読み直して
//    制御下に入れてから試す（現実の再訪問と同じ状態にする）
//
// 使い方は scripts/ui-check.mjs と同じ。preview のポートは APP= で渡す。
const { chromium } = await import(process.env.PLAYWRIGHT ?? 'playwright-core');
const WEB = process.env.WEB ?? new URL('..', import.meta.url).pathname;
const BUILD = 'VITE_API_BASE=https://liftplan-server-vjeuvyzwlq-as.a.run.app pnpm build';

const states = (page) => page.evaluate(async () => {
  const r = await navigator.serviceWorker.getRegistration();
  return { installing: r?.installing?.state ?? null, waiting: r?.waiting?.state ?? null, active: r?.active?.state ?? null };
});

// まず v1 を作る。前回の実行が dist に v2 を残していると、同じものを
// 配ることになって更新が検知されない。
execSync("sed -i 's|<title>liftplan v2</title>|<title>liftplan</title>|' index.html", { cwd: WEB });
execSync(BUILD, { cwd: WEB, stdio: 'ignore', shell: '/bin/bash' });

const browser = await chromium.launch({ executablePath: process.env.CHROME ?? '/usr/bin/chromium', args: ['--no-sandbox'] });
const page = await (await browser.newContext()).newPage();
const errs = [];
page.on('pageerror', (e) => errs.push(e.message));

await page.goto(process.env.APP ?? 'http://localhost:4173/');
await page.evaluate(() => navigator.serviceWorker.ready.then(() => undefined));
await page.waitForTimeout(1200);

// 一度読み直して SW の制御下に入れる。初回訪問のページは制御されておらず、
// 新しい SW が waiting を経ずに有効化されてしまう（現実の再訪問と違う）。
await page.reload();
await page.waitForTimeout(1500);
console.log('制御下にあるか:', await page.evaluate(() => navigator.serviceWorker.controller !== null));
console.log('SW 登録数（トークン未入力）:', await page.evaluate(async () => (await navigator.serviceWorker.getRegistrations()).length));
console.log('v1 の状態:', JSON.stringify(await states(page)));
console.log('バナー（まだ出ないはず）:', await page.locator('text=新しい版があります').count());

console.log('\n--- v2 をビルド ---');
// コメントを足しても minify で消えて sw.js が1バイトも変わらない。
// 実際に出力が変わる変更（title）にする。
execSync("sed -i 's|<title>liftplan</title>|<title>liftplan v2</title>|' index.html", { cwd: WEB });
execSync(BUILD, { cwd: WEB, stdio: 'ignore', shell: '/bin/bash' });
console.log('完了');

await page.evaluate(async () => {
  const r = await navigator.serviceWorker.getRegistration();
  await r?.update();
});

let shown = 0;
for (let i = 0; i < 25; i++) {
  await page.waitForTimeout(1000);
  shown = await page.locator('text=新しい版があります').count();
  if (i % 5 === 0 || shown) console.log(`  ${i}s 状態=${JSON.stringify(await states(page))} バナー=${shown}`);
  if (shown) break;
}

console.log('\nバナー（出るべき）:', shown);
if (shown) {
  console.log('押す前のタイトル:', await page.title());
  await page.click('text=新しい版にする');
  await page.waitForTimeout(8000);
  console.log('押したあとのタイトル:', await page.title(), '（v2 になっていれば新版が有効）');
  console.log('押したあと: バナー=', await page.locator('text=新しい版があります').count(), '状態=', JSON.stringify(await states(page)));
}
console.log('エラー:', errs.length ? errs.join('\n') : '(なし)');
// 後片付け。index.html を戻し、dist も v1 に戻しておく。
execSync("sed -i 's|<title>liftplan v2</title>|<title>liftplan</title>|' index.html", { cwd: WEB });
execSync(BUILD, { cwd: WEB, stdio: 'ignore', shell: '/bin/bash' });
await browser.close();
process.exit(shown ? 0 : 1);
