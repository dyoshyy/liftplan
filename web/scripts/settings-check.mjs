// 設定の保存を実機で確かめる。
//
// **週に通う回数が一番影響が大きい。**以前は保存のたびに設定を取り直して
// いて、そのせいで画面が空白になった（描画時の program を掴んだ関数が
// 古い値を見て抜けていた）。単体テストでは踏めない。
//
// 種目は押したその場で保存する。押しただけで本当にサーバーへ届くか、
// 届いた値が下の候補と畳んだ要約に反映されるかも、配線なのでここで見る。
//
// 使い方は scripts/ui-check.mjs と同じ。ポートは APP= と API= で渡す。
const { chromium } = await import(process.env.PLAYWRIGHT ?? 'playwright-core');
const APP = process.env.APP ?? 'http://localhost:4173';
const API = process.env.API ?? 'http://127.0.0.1:8080';
const TOKEN = 'dev-token-0123456789abcdef0123456789ab';
const auth = { Authorization: `Bearer ${TOKEN}` };
const program = () => fetch(`${API}/api/program`, { headers: auth }).then((r) => r.json());

const before = await program();
// 開発用のセッションはアカウントを持たないので、本物の応答は空になる。
// 空であることをここで見てから、画面へは作った応答を渡す。アドレスが
// 出る側の配線は、実際に出る応答でしか確かめられない。
const realAccount = await fetch(`${API}/api/account`, { headers: auth }).then((r) => r.json());
console.log('本物の /api/account:', JSON.stringify(realAccount), '（期待 {"accounts":[]}）');
console.log('保存前: per_week =', before.per_week, '/ declared =', before.declared_exercises.length, '件');

const browser = await chromium.launch({ executablePath: process.env.CHROME ?? '/usr/bin/chromium', args: ['--no-sandbox'] });
const page = await (await browser.newContext({ viewport: { width: 390, height: 844 } })).newPage();
const errs = [];
page.on('pageerror', (e) => errs.push(e.message));
page.on('console', (m) => { if (m.type() === 'error') errs.push(m.text()); });
await page.route('**/api/account', (route) => route.fulfill({
  contentType: 'application/json',
  body: JSON.stringify({ accounts: [
    { provider: 'github', email: 'gym@example.com' },
    { provider: 'google', email: 'gym@example.com' },
  ] }),
}));

// ログインはフラグメントで済ませる。/auth/* を通すとプロバイダの画面が
// 挟まり、自動では抜けられない。#token= はサーバーがコールバックで戻して
// くる形そのものなので、取り込みの配線もここで一緒に検査できる。
await page.goto(`${APP}/#token=${TOKEN}`);
await page.waitForTimeout(2500);
await page.click('header button[aria-label="設定"]');
await page.waitForTimeout(1800);

// 週に通う回数を変える。「通い方」は最初から開いている。
const want = before.per_week === 3 ? 4 : 3;
// aria-label で指す。'select' だと先頭の1つを掴むので、選択が増えると
// 別の設定を黙って触る。
const FREQ = 'select[aria-label="週に通う回数"]';
await page.selectOption(FREQ, String(want));
await page.waitForTimeout(2500);

const after = await program();
console.log('保存後: per_week =', after.per_week, '（期待', want, '）');
console.log('他が壊れていないか: declared =', after.declared_exercises.length, '件 / selected =', after.selected_exercises.length, '件');

// 元に戻す
await page.selectOption(FREQ, String(before.per_week));
await page.waitForTimeout(2000);
const restored = await program();
console.log('戻した後: per_week =', restored.per_week);

// 1回の量。片方の選択を変えたとき、もう片方が送られずに 0 で断られる
// 配線ミス（400）もここで出る。
const EX = 'select[aria-label="1回の種目数"]';
const wantEx = restored.exercises_per_session === 4 ? 5 : 4;
await page.selectOption(EX, String(wantEx));
await page.waitForTimeout(2500);
const vol = await program();
console.log('1回の量: ', vol.exercises_per_session, '種目 ×', vol.sets_per_exercise, 'セット（期待', wantEx, '種目 ×', restored.sets_per_exercise, 'セット）');
await page.selectOption(EX, String(restored.exercises_per_session));
await page.waitForTimeout(2000);
const volRestored = await program();

// 種目。「使う種目」のチェックを1つ外すだけで保存されるかを見る。初期状態は
// 全種目が選ばれているので、入れるのではなく外す。伸ばしたい種目は外せない
// （disabled）ので、押せるものから選ぶ。節は畳んであるので開いてから触る。
// 見出しは aria-expanded で指す。種目のボタンと取り違えない。
const exHeader = page.locator('button[aria-expanded]', { hasText: /^種目/ });
await exHeader.click();
await page.waitForTimeout(500);
const useGroup = page.getByRole('group', { name: '使う種目' });
const growGroup = page.getByRole('group', { name: '伸ばしたい種目' });
const toUnpick = useGroup.locator('button:not([disabled])').filter({ hasText: '✓' }).first();
const unpickedName = (await toUnpick.textContent())?.replace('✓', '').trim();
await toUnpick.click();
await page.waitForTimeout(1500);
const unpicked = await program();
// 外した種目は、下の「伸ばしたい種目」の候補からも消えているはず。
const leftInGrow = await growGroup.locator('button', { hasText: unpickedName }).count();
await exHeader.click();
await page.waitForTimeout(300);
const summary = (await exHeader.textContent()) ?? '';
console.log('外した種目（', unpickedName, '）: selected =', unpicked.selected_exercises.length, '件（期待', before.selected_exercises.length - 1,
  '）/ 伸ばしたいの候補に残った数 =', leftInGrow, '（期待 0）/ 畳んだ要約 =', summary);

// 入れ直して元に戻す。
await exHeader.click();
await page.waitForTimeout(300);
await useGroup.locator('button', { hasText: unpickedName }).click();
await page.waitForTimeout(1500);
const repicked = await program();
console.log('入れ直した後: selected =', repicked.selected_exercises.length, '件');

// 種目は別ページ（設定 → 種目 → 種目を管理する）で足す・直す・消す。
// 押したその場で送るので、送った後に一覧（種目マスタ）を取り直さないと
// 画面に反映されない。配線なのでここで見る。
const exercises = () => fetch(`${API}/api/exercises`, { headers: auth }).then((r) => r.json());
const CUSTOM = 'チェック用マシン';
const RENAMED = 'チェック用マシン（直した）';

// 「種目」は前の操作で開いたまま。種目マスタの一覧はこの節の中の
// ボタンから別ページへ移る。
await page.getByRole('button', { name: '種目を管理する' }).click();
await page.waitForTimeout(1200);
const addTop = page.getByRole('button', { name: '種目を足す', exact: true });
const exercisesPageOpened = (await addTop.count()) === 1;
console.log('種目のページが開いた:', exercisesPageOpened);

// 一覧の最初の行の「直す」「消す」が画面の外に押し出されていないか。
// 寄与の要約（例：「大胸筋下部 1.0・上腕三頭筋外側頭 0.6・…」）が折り返さずに
// 1行の幅として行を広げると、scrollWidth は innerWidth のままなのに（どこかで
// overflow:hidden により切られるだけで）ボタンには実機で指が届かなくなる。
// 横スクロールの有無ではなく、ボタンの実座標が 390px の中にあるかで見る。
const firstEdit = page.getByRole('button', { name: /を直す$/ }).first();
const firstDelete = page.getByRole('button', { name: /を消す$/ }).first();
const editBox = await firstEdit.boundingBox();
const deleteBox = await firstDelete.boundingBox();
const rowButtonsVisible =
  !!editBox && !!deleteBox && editBox.x + editBox.width <= 390 && deleteBox.x + deleteBox.width <= 390;
console.log(
  '一覧1行目の直す/消すが390px内:',
  rowButtonsVisible,
  '（直す right =', editBox ? Math.round(editBox.x + editBox.width) : null,
  '/ 消す right =', deleteBox ? Math.round(deleteBox.x + deleteBox.width) : null, '）',
);

await addTop.click();
await page.waitForTimeout(300);
const editGroup = page.getByRole('group', { name: '種目の編集' });
await editGroup.getByLabel('名前').fill(CUSTOM);
await editGroup.getByRole('button', { name: '僧帽筋中部', exact: true }).click(); // 1.0
await editGroup.getByRole('button', { name: '広背筋', exact: true }).click(); // 1.0 → 下の数値欄で 0.5 に直す
await editGroup.getByLabel('広背筋の寄与').fill('0.5');
await editGroup.getByRole('button', { name: '保存' }).click();
await page.waitForTimeout(2000);

const addedList = (await exercises()).exercises;
const added = addedList.find((e) => e.name === CUSTOM);
const inList = await page.getByText(CUSTOM, { exact: true }).count();
console.log('足した種目:', JSON.stringify(added ? { stimulus: added.stimulus } : null), '/ 一覧に出た数 =', inList);

// 直す。同じ編集フォームが、押した種目の値で開くはず。
await page.getByRole('button', { name: `${CUSTOM}を直す` }).click();
await page.waitForTimeout(300);
await editGroup.getByLabel('名前').fill(RENAMED);
await editGroup.getByRole('button', { name: '保存' }).click();
await page.waitForTimeout(2000);

const afterEditList = (await exercises()).exercises;
const edited = afterEditList.find((e) => e.id === added?.id);
const oldNameGone = (await page.getByText(CUSTOM, { exact: true }).count()) === 0;
const newNameThere = (await page.getByText(RENAMED, { exact: true }).count()) === 1;
console.log('直した後: name =', edited?.name, '（期待', RENAMED, '）/ 古い名前が消えた', oldNameGone, '/ 新しい名前が出た', newNameThere);

// 消す。
await page.getByRole('button', { name: `${RENAMED}を消す` }).click();
await page.waitForTimeout(2000);
const deleted = (await exercises()).exercises.find((e) => e.id === added?.id);
const removedFromList = (await page.getByText(RENAMED, { exact: true }).count()) === 0;
console.log('消した後: deleted =', deleted?.deleted, '/ 一覧から消えた', removedFromList);

const customOk = added !== undefined && added.stimulus.TRAP_MID === 1 && added.stimulus.LAT === 0.5
  && inList === 1 && edited?.name === RENAMED && oldNameGone && newNameThere
  && deleted?.deleted === true && removedFromList;

// プリセット由来も消せる。サイドレイズは既定では伸ばしたい種目に入って
// いない（seed.DefaultDeclared）ので、409 を踏まずに消せるはず。
const PRESET = 'サイドレイズ';
await page.getByRole('button', { name: `${PRESET}を消す`, exact: true }).click();
await page.waitForTimeout(2000);
const presetDeleted = (await exercises()).exercises.find((e) => e.name === PRESET);
const presetGoneFromList = (await page.getByText(PRESET, { exact: true }).count()) === 0;
console.log('プリセットを消した後: deleted =', presetDeleted?.deleted, '/ 一覧から消えた', presetGoneFromList);

// 設定へ戻り、「使う種目」からも消えているかを見る。消した分を API で
// 足し直す必要は無い（インメモリなので再起動で戻る）。
await page.getByRole('button', { name: '← 設定' }).click();
await page.waitForTimeout(1800);
await exHeader.click();
await page.waitForTimeout(800);
// hasText の文字列指定は部分一致なので、素の PRESET だとサイドレイズも
// ケーブルサイドレイズも同じヒットになる。ここで見たいのは前者だけが
// 消えたことなので、行末に固定して区別する。
const presetGoneFromUse = (await useGroup.locator('button', { hasText: new RegExp(`^✓?\\s*${PRESET}$`) }).count()) === 0;
console.log('設定の使う種目から消えた:', presetGoneFromUse);

const presetOk = presetDeleted?.deleted === true && presetGoneFromList && presetGoneFromUse;
await exHeader.click();
await page.waitForTimeout(300);

// アカウント。畳んだ見出しにアドレス、開くとログイン方法が出るか。
const accountHeader = page.locator('button[aria-expanded]', { hasText: /^アカウント/ });
const accountSummary = (await accountHeader.textContent()) ?? '';
await accountHeader.click();
await page.waitForTimeout(300);
const signedInAs = await page.getByText('gym@example.com（GitHub・Google）でログインしています').isVisible();
console.log('アカウント: 畳んだ見出し =', accountSummary, '/ 開いた中の1行', signedInAs, '（期待 true）');

console.log('エラー:', errs.length ? errs.join('\n') : '(なし)');
const ok = after.per_week === want && after.declared_exercises.length === before.declared_exercises.length && restored.per_week === before.per_week
  && vol.exercises_per_session === wantEx && vol.sets_per_exercise === restored.sets_per_exercise
  && volRestored.exercises_per_session === restored.exercises_per_session
  && after.selected_exercises.length === before.selected_exercises.length
  && unpicked.selected_exercises.length === before.selected_exercises.length - 1
  && leftInGrow === 0
  && summary.includes(`使う${before.selected_exercises.length - 1}・`)
  && repicked.selected_exercises.length === before.selected_exercises.length
  && Array.isArray(realAccount.accounts) && realAccount.accounts.length === 0
  && accountSummary.includes('gym@example.com') && signedInAs
  && exercisesPageOpened && rowButtonsVisible && customOk && presetOk;
console.log(ok ? '\n✓ 設定の保存は壊れていない' : '\n✗ 壊れている');
await browser.close();
process.exit(ok ? 0 : 1);
