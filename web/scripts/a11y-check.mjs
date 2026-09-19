// 画面を計測する。目分量ではなく数字で見る。
//
// 見るもの:
//   - 指で押す的が 44x44 未満でないか
//   - 文字のコントラストが WCAG AA（本文 4.5 / 大きい字 3.0）を満たすか
//   - 横スクロールが出ていないか
//   - 下端の固定バーに隠れて押せない要素が無いか
//   - 文字を持たないボタンに読み上げ名があるか
//
// 一度これで、**コントラスト 2.80 の文字が今日の画面に27箇所**あることと、
// 設定への唯一の入口である歯車が 28x28 だったことが分かった。どちらも
// 目で見ている限り気づけなかった。
//
// 使い方は scripts/ui-check.mjs と同じ。ポートは APP= と API= で渡す。
const { chromium } = await import(process.env.PLAYWRIGHT ?? 'playwright-core');
const APP = process.env.APP ?? 'http://localhost:4173';
const TOKEN = 'dev-token-0123456789abcdef0123456789ab';

const browser = await chromium.launch({ executablePath: process.env.CHROME ?? '/usr/bin/chromium', args: ['--no-sandbox'] });
// iPhone SE 相当。ジムで片手で持つ一番小さい部類。
const ctx = await browser.newContext({ viewport: { width: 375, height: 667 }, deviceScaleFactor: 2 });
const page = await ctx.newPage();
const errs = [];
page.on('pageerror', (e) => errs.push('pageerror: ' + e.message));
page.on('console', (m) => { if (m.type() === 'error') errs.push(m.text()); });

const audit = async (label) => page.evaluate((label) => {
  const lum = (c) => {
    const [r, g, b] = c.match(/\d+(\.\d+)?/g).slice(0, 3).map(Number).map((v) => {
      const s = v / 255;
      return s <= 0.03928 ? s / 12.92 : ((s + 0.055) / 1.055) ** 2.4;
    });
    return 0.2126 * r + 0.7152 * g + 0.0722 * b;
  };
  const ratio = (a, b) => { const [x, y] = [lum(a), lum(b)].sort((p, q) => q - p); return (x + 0.05) / (y + 0.05); };
  const bgOf = (el) => {
    for (let n = el; n; n = n.parentElement) {
      const bg = getComputedStyle(n).backgroundColor;
      if (bg && !/rgba?\([^)]*,\s*0\)/.test(bg)) return bg;
    }
    return 'rgb(14,18,22)';
  };

  const out = { label, small: [], lowContrast: [], overflowX: 0, occluded: [], noAria: [] };

  // 1. 指で押す的が小さいもの（44x44 が目安）
  for (const el of document.querySelectorAll('button, a, input, select')) {
    const r = el.getBoundingClientRect();
    if (r.width === 0 || r.height === 0) continue;
    if (r.height < 44 || r.width < 44) {
      out.small.push({ t: (el.textContent || el.getAttribute('aria-label') || el.tagName).trim().slice(0, 22), w: Math.round(r.width), h: Math.round(r.height) });
    }
  }

  // 2. 文字のコントラスト（AA は本文 4.5、大きい字 3.0）
  for (const el of document.querySelectorAll('p, span, label, button, li, h1, h2, div')) {
    if (!el.childNodes.length) continue;
    const text = [...el.childNodes].filter((n) => n.nodeType === 3).map((n) => n.textContent.trim()).join('');
    if (!text) continue;
    const st = getComputedStyle(el);
    const size = parseFloat(st.fontSize);
    const big = size >= 24 || (size >= 18.66 && Number(st.fontWeight) >= 700);
    const r = ratio(st.color, bgOf(el));
    if (r < (big ? 3 : 4.5)) {
      out.lowContrast.push({ t: text.slice(0, 26), size: Math.round(size), ratio: r.toFixed(2), need: big ? 3 : 4.5 });
    }
  }

  // 3. 横スクロール
  out.overflowX = Math.max(0, document.documentElement.scrollWidth - document.documentElement.clientWidth);

  // 4. 下端の固定バーに隠れて押せないもの
  const bars = [...document.querySelectorAll('nav, div.fixed')].map((b) => b.getBoundingClientRect()).filter((r) => r.height > 0);
  for (const el of document.querySelectorAll('button, input')) {
    const r = el.getBoundingClientRect();
    if (r.height === 0 || r.bottom < 0 || r.top > innerHeight) continue;
    for (const b of bars) {
      if (r.top < b.bottom && r.bottom > b.top && !b.height === 0) {
        const hit = document.elementFromPoint(r.left + r.width / 2, r.top + r.height / 2);
        if (hit && !el.contains(hit) && !hit.contains(el)) {
          out.occluded.push({ t: (el.textContent || el.tagName).trim().slice(0, 20) });
        }
        break;
      }
    }
  }

  // 5. 文字を持たないボタンに読み上げ名が無い
  for (const el of document.querySelectorAll('button')) {
    if (!el.textContent.trim() && !el.getAttribute('aria-label') && !el.getAttribute('title')) {
      out.noAria.push(el.outerHTML.slice(0, 60));
    }
  }
  return out;
}, label);

const report = [];
const snap = async (label, file) => {
  const a = await audit(label);
  report.push(a);
  if (file) await page.screenshot({ path: file, fullPage: false });
};

await page.goto(APP);
await page.waitForTimeout(1500);
await snap('設定（トークン入力）', '/tmp/a-setup.png');

await page.fill('input[type=password]', TOKEN);
await page.click('text=保存する');
await page.waitForTimeout(3000);
await snap('今日', '/tmp/a-today.png');

await page.click('nav >> text=履歴');
await page.waitForTimeout(2500);
await snap('履歴', '/tmp/a-history.png');

await page.click('header button[aria-label="設定"]');
await page.waitForTimeout(2500);
await snap('設定', '/tmp/a-settings.png');

await page.goBack();
await page.waitForTimeout(1200);
await page.locator('button.set').first().click();
await page.waitForTimeout(800);
await snap('記録シート', '/tmp/a-sheet.png');

console.log(JSON.stringify(report, null, 1));
console.log('\nコンソールエラー:', errs.length ? errs.join('\n') : '(なし)');
await browser.close();
