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

const browser = await chromium.launch({
  executablePath: process.env.CHROME ?? '/usr/bin/chromium',
  args: ['--no-sandbox'],
});
const page = await (await browser.newContext({ viewport: { width: 390, height: 844 } })).newPage();
const errs = [];
page.on('pageerror', (e) => errs.push(e.message));
page.on('console', (m) => {
  if (m.type() === 'error') errs.push(m.text());
});
await page.route('**/api/account', (route) =>
  route.fulfill({
    contentType: 'application/json',
    body: JSON.stringify({
      accounts: [
        { provider: 'github', email: 'gym@example.com' },
        { provider: 'google', email: 'gym@example.com' },
      ],
    }),
  }),
);

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
console.log(
  '他が壊れていないか: declared =',
  after.declared_exercises.length,
  '件 / selected =',
  after.selected_exercises.length,
  '件',
);

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
console.log(
  '1回の量: ',
  vol.exercises_per_session,
  '種目 ×',
  vol.sets_per_exercise,
  'セット（期待',
  wantEx,
  '種目 ×',
  restored.sets_per_exercise,
  'セット）',
);
await page.selectOption(EX, String(restored.exercises_per_session));
await page.waitForTimeout(2000);
const volRestored = await program();

// 種目。使う種目の入れ替えは設定のトップではなく、種目ページ（設定 → 種目 →
// 種目を管理する）でする。設定のトップに使わない種目が並ばないため。
// 節は畳んであるので開いてから触る。見出しは aria-expanded で指す。
const exHeader = page.locator('button[aria-expanded]', { hasText: /^種目/ });
await exHeader.click();
await page.waitForTimeout(500);
const growGroup = page.getByRole('group', { name: '伸ばしたい種目' });
const settingsHasUseGroup = (await page.getByRole('group', { name: '使う種目' }).count()) > 0;
console.log('設定のトップに「使う種目」の一覧が出ない:', !settingsHasUseGroup, '（期待 true）');

// 種目は別ページ（設定 → 種目 → 種目を管理する）で足す・直す・消す。
// 押したその場で送るので、送った後に一覧（種目マスタ）を取り直さないと
// 画面に反映されない。配線なのでここで見る。
const exercises = () => fetch(`${API}/api/exercises`, { headers: auth }).then((r) => r.json());
const CUSTOM = 'チェック用マシン';
const RENAMED = 'チェック用マシン（直した）';
// プリセット由来も消せることの確認に使う。既定では伸ばしたい種目に
// 入っていない（seed.DefaultDeclared）ので、409 を踏まずに消せるはず。
const PRESET = 'サイドレイズ';

// 「種目」は前の操作で開いたまま。種目マスタの一覧はこの節の中の
// ボタンから別ページへ移る。
await page.getByRole('button', { name: '種目を管理する' }).click();
await page.waitForTimeout(1200);
const addTop = page.getByRole('button', { name: '種目を追加', exact: true });
const exercisesPageOpened = (await addTop.count()) === 1;
console.log('種目のページが開いた:', exercisesPageOpened);

// 一覧の最初の行の「編集」「使う」が画面の外に押し出されていないか。
// 寄与の要約（例：「大胸筋下部 1.0・上腕三頭筋外側頭 0.6・…」）が折り返さずに
// 1行の幅として行を広げると、scrollWidth は innerWidth のままなのに（どこかで
// overflow:hidden により切られるだけで）ボタンには実機で指が届かなくなる。
// 横スクロールの有無ではなく、ボタンの実座標が 390px の中にあるかで見る。
const firstEdit = page.getByRole('button', { name: /を編集$/ }).first();
const firstUse = page.getByRole('button', { name: /を使う$/ }).first();
const editBox = await firstEdit.boundingBox();
const useBox = await firstUse.boundingBox();
const rowButtonsVisible =
  !!editBox && !!useBox && editBox.x + editBox.width <= 390 && useBox.x + useBox.width <= 390;
console.log(
  '一覧1行目の編集/使うが390px内:',
  rowButtonsVisible,
  '（編集 right =',
  editBox ? Math.round(editBox.x + editBox.width) : null,
  '/ 使う right =',
  useBox ? Math.round(useBox.x + useBox.width) : null,
  '）',
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
// 足すと使う種目にも入る（サーバーが selected_exercises に足す）。ここは
// 種目マスタ（/api/exercises）とは別の口（/api/program）で見る。
const afterAdd = await program();
const addedInUse = added !== undefined && afterAdd.selected_exercises.includes(added.id);
console.log(
  '足した種目:',
  JSON.stringify(added ? { stimulus: added.stimulus } : null),
  '/ 一覧に出た数 =',
  inList,
  '/ 使う種目に入った',
  addedInUse,
);

// 直す。同じ編集フォームが、押した種目の値で開くはず。
await page.getByRole('button', { name: `${CUSTOM}を編集` }).click();
await page.waitForTimeout(300);
await editGroup.getByLabel('名前').fill(RENAMED);
await editGroup.getByRole('button', { name: '保存' }).click();
await page.waitForTimeout(2000);

const afterEditList = (await exercises()).exercises;
const edited = afterEditList.find((e) => e.id === added?.id);
const oldNameGone = (await page.getByText(CUSTOM, { exact: true }).count()) === 0;
const newNameThere = (await page.getByText(RENAMED, { exact: true }).count()) === 1;
console.log(
  '直した後: name =',
  edited?.name,
  '（期待',
  RENAMED,
  '）/ 古い名前が消えた',
  oldNameGone,
  '/ 新しい名前が出た',
  newNameThere,
);

// 消す口は無い。使う種目から外す（使わない）と、計画にも出なくなる。
// 「使う」ボタンは aria-pressed で状態を持つ。
const toggleOf = (name) => page.getByRole('button', { name: `${name}を使う`, exact: true });
const pressed = async (name) => (await toggleOf(name).getAttribute('aria-pressed')) === 'true';

const noDeleteButton = (await page.getByRole('button', { name: /削除/ }).count()) === 0;
console.log('削除のボタンが無い:', noDeleteButton, '（期待 true）');

const customInUseBefore = await pressed(RENAMED);
await toggleOf(RENAMED).click();
await page.waitForTimeout(1500);
const afterHide = await program();
const customHidden = added !== undefined && !afterHide.selected_exercises.includes(added.id);
const stillThere = (await exercises()).exercises.find((e) => e.id === added?.id);
console.log(
  '使わないにした: 前は使う',
  customInUseBefore,
  '/ selected から外れた',
  customHidden,
  '/ 種目そのものは残る deleted =',
  stillThere?.deleted,
  '（期待 undefined か false）',
);
await toggleOf(RENAMED).click();
await page.waitForTimeout(1500);
const afterShow = await program();
const customShown = added !== undefined && afterShow.selected_exercises.includes(added.id);
console.log('また使うにした: selected に戻った', customShown);

// プリセット由来も同じ。既定では伸ばしたい種目に入っていない
// （seed.DefaultDeclared）ので、外せるはず。
const presetInUseBefore = await pressed(PRESET);
await toggleOf(PRESET).click();
await page.waitForTimeout(1500);
const presetId = (await exercises()).exercises.find((e) => e.name === PRESET)?.id;
const presetHidden = !(await program()).selected_exercises.includes(presetId);
console.log('プリセットを使わないにした: 前は使う', presetInUseBefore, '/ selected から外れた', presetHidden);

// 伸ばしたい種目は外せない。理由が出て、押せない。
const BENCH = 'ベンチプレス';
const benchLocked = await toggleOf(BENCH).isDisabled();
console.log('伸ばしたい種目（ベンチプレス）は外せない（disabled）:', benchLocked, '（期待 true）');

// 設定へ戻ると、外した種目は「伸ばしたい種目」の候補から消えている。
await page.locator('main').getByRole('button', { name: '設定', exact: true }).click();
await page.waitForTimeout(1800);
await exHeader.click();
await page.waitForTimeout(800);
const leftInGrow = await growGroup.locator('button', { hasText: new RegExp(`^✓?\\s*${PRESET}$`) }).count();
console.log('伸ばしたい種目の候補からも消えた:', leftInGrow === 0, '（残った数 =', leftInGrow, '）');
const summary = (await exHeader.textContent()) ?? '';
console.log('畳んだ要約 =', summary);

// 元に戻す（インメモリでも、続けて回す検査のため）。
await page.getByRole('button', { name: '種目を管理する' }).click();
await page.waitForTimeout(1200);
await toggleOf(PRESET).click();
await page.waitForTimeout(1500);
const restoredSelected = (await program()).selected_exercises.length;
await page.locator('main').getByRole('button', { name: '設定', exact: true }).click();
await page.waitForTimeout(1500);

const customOk =
  added !== undefined &&
  added.stimulus.TRAP_MID === 1 &&
  added.stimulus.LAT === 0.5 &&
  inList === 1 &&
  addedInUse &&
  edited?.name === RENAMED &&
  oldNameGone &&
  newNameThere &&
  noDeleteButton &&
  customInUseBefore &&
  customHidden &&
  stillThere?.deleted !== true &&
  customShown;
const presetOk =
  presetInUseBefore &&
  presetHidden &&
  benchLocked &&
  leftInGrow === 0 &&
  restoredSelected === before.selected_exercises.length + 1;
// 宣言ごとのレップ数。選んだその場でサーバーに届くか、宣言を足すと行が出るか
// （取り直した declared_reps が画面に届くか）。種目ページから戻ると節は畳まれている。
if ((await exHeader.getAttribute('aria-expanded')) !== 'true') {
  await exHeader.click();
  await page.waitForTimeout(500);
}
const repsErrs = [];
const HEAVY = 'select[aria-label$="の重い日のレップ数"]';
const repsBefore = await program();
const firstDeclared = repsBefore.declared_exercises[0];
const heavyBefore = repsBefore.declared_reps[firstDeclared].heavy;
const heavyWant = heavyBefore === 3 ? 8 : 3;
await page.locator(HEAVY).first().selectOption(String(heavyWant));
await page.waitForTimeout(1200);
const repsAfter = await program();
console.log(
  '重い日のレップ数:',
  heavyBefore,
  '→',
  repsAfter.declared_reps[firstDeclared].heavy,
  `（期待 ${heavyWant}）`,
);
if (repsAfter.declared_reps[firstDeclared].heavy !== heavyWant)
  repsErrs.push('重い日のレップ数が保存されていない');
// 画面の値がサーバーの値のまま残るか（手元の書き換えが抜けると選択が戻る）。
const shownHeavy = await page.locator(HEAVY).first().inputValue();
console.log('画面の重い日のレップ数:', shownHeavy, `（期待 ${heavyWant}）`);
if (shownHeavy !== String(heavyWant)) repsErrs.push('選んだ重い日のレップ数が画面に残らない');
// 他の宣言は動かない。
for (const id of repsBefore.declared_exercises.slice(1)) {
  if (JSON.stringify(repsAfter.declared_reps[id]) !== JSON.stringify(repsBefore.declared_reps[id])) {
    repsErrs.push(`${id} のレップ数が動いた`);
  }
}
// 元に戻す。
await page.locator(HEAVY).first().selectOption(String(heavyBefore));
await page.waitForTimeout(1200);

// 重点種目を選ぶと軽い日の行が出て、選んだ値が届く。重い日は動かない。
const exNames = new Map((await exercises()).exercises.map((e) => [e.id, e.name]));
const firstName = exNames.get(firstDeclared);
const LIGHT = `select[aria-label="${firstName}の軽い日のレップ数"]`;
const lightRowsBefore = await page.locator(LIGHT).count();
await page.locator('button', { hasText: new RegExp(`^${firstName}$`) }).click();
await page.waitForTimeout(1200);
const lightRowsFocused = await page.locator(LIGHT).count();
const lightWant = repsBefore.declared_reps[firstDeclared].light === 6 ? 9 : 6;
await page.locator(LIGHT).selectOption(String(lightWant));
await page.waitForTimeout(1200);
const lightAfter = (await program()).declared_reps[firstDeclared];
console.log(
  '軽い日のレップ数: 行',
  lightRowsBefore,
  '→ 重点で',
  lightRowsFocused,
  '/ サーバー',
  lightAfter.light,
  `（期待 ${lightWant}）/ 重い日`,
  lightAfter.heavy,
  `（期待 ${heavyBefore}）`,
);
if (lightRowsBefore !== 0 || lightRowsFocused !== 1)
  repsErrs.push('軽い日の行が重点種目に付いて出入りしない');
if (lightAfter.light !== lightWant || lightAfter.heavy !== heavyBefore)
  repsErrs.push('軽い日のレップ数が保存されていない');
// 重点を外す。
await page.getByRole('button', { name: '指定しない', exact: true }).click();
await page.waitForTimeout(1200);
if ((await page.locator(LIGHT).count()) !== 0) repsErrs.push('重点を外しても軽い日の行が残る');

// 宣言を足すと、その種目の重い日の行が出る。宣言していない「使う種目」を
// 1つ押して行を数え、押し直して元に戻す。
const rowsBefore = await page.locator(HEAVY).count();
const addable = repsBefore.selected_exercises.find((id) => !repsBefore.declared_exercises.includes(id));
const addableName = exNames.get(addable);
const addableButton = growGroup.locator('button', { hasText: new RegExp(`^✓?\\s*${addableName}$`) });
await addableButton.click();
await page.waitForTimeout(1500);
const rowsAfter = await page.locator(HEAVY).count();
const newRow = await page
  .locator(`select[aria-label="${addableName}の重い日のレップ数"]`)
  .inputValue()
  .catch(() => null);
console.log(
  '宣言を足したあとの重い日の行:',
  rowsBefore,
  '→',
  rowsAfter,
  `（期待 ${rowsBefore + 1}）/ 既定の値 =`,
  newRow,
);
if (rowsAfter !== rowsBefore + 1) repsErrs.push('宣言を足しても重い日の行が出ない');
await addableButton.click();
await page.waitForTimeout(1500);
const rowsRestored = await page.locator(HEAVY).count();
console.log('宣言を外したあとの重い日の行:', rowsRestored, `（期待 ${rowsBefore}）`);
if (rowsRestored !== rowsBefore) repsErrs.push('宣言を外しても重い日の行が残る');
const finalProgram = await program();
if (JSON.stringify(finalProgram.declared_exercises) !== JSON.stringify(repsBefore.declared_exercises)) {
  repsErrs.push('宣言が元に戻っていない');
}
console.log('レップ数の検査のエラー:', repsErrs.length ? repsErrs.join(' / ') : '(なし)');

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
const ok =
  after.per_week === want &&
  after.declared_exercises.length === before.declared_exercises.length &&
  restored.per_week === before.per_week &&
  vol.exercises_per_session === wantEx &&
  vol.sets_per_exercise === restored.sets_per_exercise &&
  volRestored.exercises_per_session === restored.exercises_per_session &&
  after.selected_exercises.length === before.selected_exercises.length &&
  !settingsHasUseGroup &&
  Array.isArray(realAccount.accounts) &&
  realAccount.accounts.length === 0 &&
  accountSummary.includes('gym@example.com') &&
  signedInAs &&
  exercisesPageOpened &&
  rowButtonsVisible &&
  customOk &&
  presetOk &&
  repsErrs.length === 0;
console.log(ok ? '\n✓ 設定の保存は壊れていない' : '\n✗ 壊れている');
await browser.close();
process.exit(ok ? 0 : 1);
