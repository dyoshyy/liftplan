'use strict';

// liftplan のクライアント。
//
// 設計の要点。
//
// 1. 記録を失わない。ジムの電波は途切れる。送信は待ち行列に積んでから
//    投げ、失敗しても消さない。IDはクライアントが採番するので、
//    サーバーが冪等に受け止める（同じ内容の再送は成功、内容違いは 409）。
// 2. 未来のメニューを手元に持たない。表示は常にサーバーから取り直す。
//    持つと、記録した結果が反映されているのか分からなくなる。
// 3. 「どこまでやったか」はサーバーの実績から復元する。端末の中だけに
//    持つと、画面を閉じた瞬間に分からなくなる。

const KEY_TOKEN = 'liftplan.token';
const KEY_QUEUE = 'liftplan.queue';

const $ = (id) => document.getElementById(id);
const el = (tag, cls, html) => {
  const n = document.createElement(tag);
  if (cls) n.className = cls;
  if (html !== undefined) n.innerHTML = html;
  return n;
};
const esc = (s) => String(s).replace(/[&<>"]/g, (c) =>
  ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;' }[c]));

function today() {
  const d = new Date();
  const p = (n) => String(n).padStart(2, '0');
  return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())}`;
}
function addDays(iso, days) {
  const d = new Date(iso + 'T00:00:00');
  d.setDate(d.getDate() + days);
  const p = (n) => String(n).padStart(2, '0');
  return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())}`;
}
function label(iso) {
  const d = new Date(iso + 'T00:00:00');
  return `${d.getMonth() + 1}/${d.getDate()} (${'日月火水木金土'[d.getDay()]})`;
}
function newId() {
  return `w-${Date.now().toString(36)}-${Math.random().toString(36).slice(2, 10)}`;
}

// 捨てた記録は端末に残す。消えたことに気づけないのが一番まずい。
const KEY_REJECTED = 'liftplan.rejected';

// 承認したデロードはその日のあいだ覚えておく。覚えないと、更新した
// とたんに指示が10%跳ね上がる。日付ごとに持つので翌日には消える。
const KEY_DELOAD = 'liftplan.deload';

const store = {
  token: () => localStorage.getItem(KEY_TOKEN) || '',
  setToken: (v) => localStorage.setItem(KEY_TOKEN, v),
  clearToken: () => localStorage.removeItem(KEY_TOKEN),
  deload: (d) => {
    try {
      const v = JSON.parse(localStorage.getItem(KEY_DELOAD) || 'null');
      return v && v.date === d ? v.ids || [] : [];
    } catch { return []; }
  },
  setDeload: (d, ids) => localStorage.setItem(KEY_DELOAD, JSON.stringify({ date: d, ids })),
};

async function api(path, options = {}) {
  const res = await fetch(path, {
    ...options,
    headers: {
      ...(options.headers || {}),
      Authorization: `Bearer ${store.token()}`,
      ...(options.body ? { 'Content-Type': 'application/json' } : {}),
    },
  });
  if (res.status === 401) {
    store.clearToken();
    showSetup('トークンが違います');
    throw new Error('unauthorized');
  }
  return res;
}

// --- 送信の待ち行列 --------------------------------------------------
//
// localStorage ではなく IndexedDB に置く。
//
// localStorage には「読んで、足して、書く」を割り込まれずに行う手段が
// 無い。携帯とPCのように同じ端末で2つ開いていると、両方が同じキーを
// 読んで書き戻し、あとから書いた方が相手の記録を消す。実際に2タブから
// 3セットずつ記録したら、6件のうち3件が消えた。
//
// IndexedDB の書き込みはトランザクションなので、追記どうしがぶつからない。
// 2つのタブが同時に送っても、二重に送るだけで済む（同じIDの同じ内容は
// サーバーが黙って受け入れる契約にしてある）。消えるよりはるかによい。

const DB_NAME = 'liftplan';
const DB_VERSION = 1;

function openDB() {
  return new Promise((resolve, reject) => {
    const req = indexedDB.open(DB_NAME, DB_VERSION);
    req.onupgradeneeded = () => {
      const db = req.result;
      if (!db.objectStoreNames.contains('queue')) {
        db.createObjectStore('queue', { autoIncrement: true });
      }
      if (!db.objectStoreNames.contains('rejected')) {
        db.createObjectStore('rejected', { autoIncrement: true });
      }
    };
    req.onsuccess = () => resolve(req.result);
    req.onerror = () => reject(req.error);
  });
}

let dbPromise = null;
function db() {
  dbPromise = dbPromise || openDB();
  return dbPromise;
}

function tx(name, mode, run) {
  return db().then((d) => new Promise((resolve, reject) => {
    const t = d.transaction(name, mode);
    const req = run(t.objectStore(name));
    t.oncomplete = () => resolve(req ? req.result : undefined);
    t.onerror = () => reject(t.error);
    t.onabort = () => reject(t.error);
  }));
}

const outbox = {
  add: (name, value) => tx(name, 'readwrite', (s) => s.add(value)),
  remove: (name, key) => tx(name, 'readwrite', (s) => s.delete(key)),
  clear: (name) => tx(name, 'readwrite', (s) => s.clear()),
  count: (name) => tx(name, 'readonly', (s) => s.count()),
  values: (name) => tx(name, 'readonly', (s) => s.getAll()),
  head: (name) => db().then((d) => new Promise((resolve, reject) => {
    const t = d.transaction(name, 'readonly');
    const cur = t.objectStore(name).openCursor();
    cur.onsuccess = () => resolve(cur.result ? { key: cur.result.key, value: cur.result.value } : null);
    cur.onerror = () => reject(cur.error);
  })),
};

// 表示は同期で描くので、件数と中身を手元にも持つ。
let pendingCount = 0;
let rejectedList = [];

// 古い版が localStorage に残したぶんを引き取る。捨てると記録が消える。
async function adoptLegacyQueue() {
  for (const [key, name] of [[KEY_QUEUE, 'queue'], [KEY_REJECTED, 'rejected']]) {
    let items = [];
    try { items = JSON.parse(localStorage.getItem(key) || '[]'); } catch { items = []; }
    if (!Array.isArray(items) || items.length === 0) continue;
    for (const it of items) await outbox.add(name, it);
    localStorage.removeItem(key);
  }
}

async function refreshOutbox() {
  pendingCount = await outbox.count('queue');
  rejectedList = await outbox.values('rejected');
}

function enqueue(item) {
  // 楽観的に数えておく。押した直後に「未送信」と出ないと、
  // 記録できたのか分からない。
  pendingCount++;
  paintStatus();
  outbox.add('queue', item)
    .then(() => flush())
    .catch(() => { pendingCount = Math.max(0, pendingCount - 1); paintStatus(); });
}

let flushing = false;

async function flush() {
  if (flushing || !store.token()) return;
  flushing = true;
  try {
    for (;;) {
      const head = await outbox.head('queue');
      if (!head) break;
      const item = head.value;

      const res = await api(item.path, {
        method: item.method || 'POST',
        body: item.body ? JSON.stringify(item.body) : undefined,
      });
      if (!res.ok && res.status >= 400 && res.status < 500) {
        // 再送しても永久に通らない。残すと後続が全部詰まる。
        // ただし黙って消すと、記録したはずのものが無いことに気づけない。
        const detail = await res.text().catch(() => '');
        console.warn('破棄', res.status, item, detail);
        await outbox.add('rejected', describeItem(item, res.status));
      } else if (!res.ok) {
        break; // 5xx / 503 はやり直せば通る
      }
      await outbox.remove('queue', head.key);
      await refreshOutbox();
      paintRejected();
    }
  } catch {
    // 電波が無い。次の機会に送る。
  } finally {
    flushing = false;
    await refreshOutbox().catch(() => {});
    paintStatus();
  }
}

// describeItem は捨てたものを人が読める1行にする。
// path と JSON をそのまま出しても、何を失ったのか分からない。
function describeItem(item, status) {
  const log = item.body?.logs?.[0];
  if (log) {
    const name = state.names?.get(log.exercise_id) || log.exercise_id;
    return `${log.date} ${name} ${log.weight_kg}kg × ${log.reps}（${status}）`;
  }
  if (item.body?.conditions?.[0]) {
    return `${item.body.conditions[0].date} のコンディション（${status}）`;
  }
  return `${item.method || 'POST'} ${item.path}（${status}）`;
}

function paintRejected() {
  const list = rejectedList;
  $('rejected-notice').classList.toggle('hidden', list.length === 0);
  $('rejected-list').innerHTML = list.map((r) => `<li>${esc(r)}</li>`).join('');
  paintStatus();
}

function paintStatus() {
  const n = pendingCount;
  const rejected = rejectedList.length;
  const dot = $('dot');
  if (rejected > 0) {
    dot.className = 'dot error';
    $('status-text').textContent = n > 0
      ? `未送信 ${n} 件・送れなかった記録 ${rejected} 件`
      : `送れなかった記録 ${rejected} 件`;
    return;
  }
  if (!navigator.onLine) {
    dot.className = 'dot error';
    $('status-text').textContent = n > 0 ? `オフライン・未送信 ${n} 件` : 'オフライン（記録は保存されます）';
    return;
  }
  dot.className = 'dot' + (n > 0 ? ' pending' : '');
  $('status-text').textContent = n > 0 ? `未送信 ${n} 件` : '同期済み';
}

// --- 状態 -----------------------------------------------------------

const state = {
  session: null,
  names: new Map(),      // exercise_id -> 日本語名
  exercises: [],
  last: {},              // exercise_id -> 前回の実績
  doneToday: new Map(),  // exercise_id -> [{id, weight, reps}]
  program: null,
  selected: new Set(),
  // いま出している画面。読み込み直しても、見ていた画面に戻る。
  view: null,
};

const nameOf = (id) => state.names.get(id) || id;

// 筋区分の日本語。表示の都合なのでここに置く。ドメインに持たせると、
// 画面の言語がドメインに漏れる。
const REGION = {
  CHEST_UPPER: '胸（上部）', CHEST_MID: '胸（中部）', CHEST_LOWER: '胸（下部）',
  LAT: '広背筋', TRAP_MID: '僧帽筋（中部）', TRAP_UPPER: '僧帽筋（上部）', ERECTOR: '脊柱起立筋',
  FRONT_DELT: '三角筋（前）', SIDE_DELT: '三角筋（横）', REAR_DELT: '三角筋（後）',
  TRICEPS_LONG: '三頭（長頭）', TRICEPS_LATERAL: '三頭（外側）',
  BICEPS: '二頭', FOREARM: '前腕',
  QUAD: '大腿四頭筋', HAMSTRING: 'ハムストリング', GLUTE: '臀筋',
  ADDUCTOR: '内転筋', CALF: 'ふくらはぎ', ABS: '腹直筋', OBLIQUE: '腹斜筋',
};
const regionName = (r) => REGION[r] || r;

// --- 今日 -----------------------------------------------------------

function exerciseCard(planned) {
  const card = el('div', 'card ex');

  const top = el('div', 'ex-top');
  top.append(el('span', 'ex-name', esc(nameOf(planned.exercise_id))));
  if (planned.role) {
    top.append(el('span', 'role' + (planned.role === 'HEAVY' ? ' heavy' : ''), esc(planned.role)));
  }
  card.append(top);

  if (planned.finished_only) {
    // 今日やったが、今の予定には入っていないもの。予定として出すと
    // 「これからやる」ように読めるので、済んだ事実だけを出す。
    card.append(el('div', 'target',
      `<span class="kg unset num">今日やった</span>` +
      `<span class="spec">${planned.sets}セット</span>`));
  } else {
    card.append(el('div', 'target', planned.weight_kg === null
      ? `<span class="kg unset num">自分で決める</span><span class="spec">${planned.sets}セット・RIR ${planned.target_rir}</span>`
      : `<span class="kg num">${planned.weight_kg}<small>kg</small></span>` +
        `<span class="spec">${planned.sets}セット・目標RIR ${planned.target_rir}</span>`));
  }

  const last = state.last[planned.exercise_id];
  if (last) {
    // 今日との差分は出さない。前回が標準日で今日が高強度日なら重量は
    // 当然変わるので、その差は「伸び」ではない。増減を要約すると
    // 「増えた＝良い」という誤った読み方を押し付けることになる。
    // 伸びているかは履歴の推定1RMの推移で見る。
    card.append(el('div', 'last',
      `前回 <span class="num">${formatLast(last)}</span>` +
      `<span class="ago">${last.days_ago}日前</span>`));
  } else {
    card.append(el('div', 'last small', '記録がまだありません'));
  }

  const recorded = state.doneToday.get(planned.exercise_id) || [];
  const sets = el('div', 'sets');
  // 予定より多く記録することはある。予定の数しか枠を出さないと、
  // はみ出したセットが画面から消えて、取り消すこともできなくなる。
  const slots = planned.finished_only ? recorded.length : Math.max(planned.sets, recorded.length);
  for (let i = 0; i < slots; i++) {
    const rec = recorded[i];
    const b = el('button', 'set' + (rec ? ' done' : ''));
    const idx = (planned.finished_only || i < planned.sets)
      ? `${i + 1}セット目` : `${i + 1}セット目・追加`;
    b.innerHTML = `<span class="idx">${idx}</span>` +
      `<span class="val">${rec ? `${rec.weight_kg}×${rec.reps}` : '記録'}</span>`;
    b.onclick = () => openSheet(planned, i, rec);
    sets.append(b);
  }
  card.append(sets);
  return card;
}

function paintToday() {
  const mains = $('mains');
  mains.innerHTML = '';
  (state.session?.main || []).forEach((p) => mains.append(exerciseCard(p)));

  const acc = $('accessories');
  acc.innerHTML = '';
  const list = state.session?.accessories || [];
  if (list.length > 0) {
    acc.append(el('p', 'card-title', '補助種目'));
    list.forEach((p) => acc.append(exerciseCard(p)));
  }

  // 今日やったのに、今の予定に入っていないものを出す。
  //
  // 補助種目は終わると枠から外れるので、セッションを終えて開き直すと
  // カードが11枚から3枚に減る。やった24セットが今日の画面から消えて、
  // 記録が飛んだように見える。
  const planned = new Set([
    ...(state.session?.main || []).map((p) => p.exercise_id),
    ...list.map((p) => p.exercise_id),
  ]);
  const leftovers = [...state.doneToday.entries()]
    .filter(([id, sets]) => !planned.has(id) && sets.length > 0)
    .sort((a, b) => (a[0] < b[0] ? -1 : 1));
  if (leftovers.length > 0) {
    acc.append(el('p', 'card-title', '今日やったもの'));
    leftovers.forEach(([id, sets]) => acc.append(exerciseCard({
      exercise_id: id, weight_kg: null, sets: sets.length,
      target_rir: null, finished_only: true,
    })));
  }

  const d = state.session?.deload_proposal;
  const stalled = d?.stalled_exercises || [];
  const accepted = store.deload(today());
  if (accepted.length > 0) {
    // 承認済み。同じ提案を出し続けると、効いたのかどうか分からない。
    $('deload').classList.remove('hidden');
    $('deload-reason').textContent =
      `${accepted.map(nameOf).join('・')}を落として組み直しました。`;
    $('accept-deload').textContent = '元に戻す';
    $('accept-deload').onclick = () => { store.setDeload(today(), []); loadToday(); };
  } else if (stalled.length > 0) {
    $('deload').classList.remove('hidden');
    $('deload-reason').textContent = d.reason;
    $('accept-deload').textContent =
      `${stalled.map(nameOf).join('・')}を${Math.round(d.intensity_drop_pct * 100)}%落とす`;
    $('accept-deload').onclick = () => {
      store.setDeload(today(), stalled);
      loadToday(stalled);
    };
  } else {
    $('deload').classList.add('hidden');
  }
}

// --- 記録シート ------------------------------------------------------

let sheetTarget = null;

function openSheet(planned, index, recorded) {
  sheetTarget = { planned, index, recorded };
  $('sheet-title').textContent = `${nameOf(planned.exercise_id)} ${index + 1}セット目`;

  const last = state.last[planned.exercise_id];
  $('sheet-last').textContent = last
    ? `前回 ${last.weights?.[index] ?? last.weight_kg}kg × ${last.reps[index] ?? '–'}` : '';

  $('w').value = recorded ? recorded.weight_kg : (planned.weight_kg ?? last?.weight_kg ?? '');
  $('reps').value = recorded ? recorded.reps : (last?.reps[index] ?? 8);
  $('rir').value = recorded ? recorded.rir : planned.target_rir;
  $('undo').classList.toggle('hidden', !recorded);
  $('sheet-warn').classList.add('hidden');
  $('sheet').showModal();
}

function closeSheet() { $('sheet').close(); sheetTarget = null; }

// warnInSheet はシートの中に注意を出す。
//
// alert() だと入力中の画面が消えたうえ、ダイアログを閉じるまで
// ページ全体が止まる。直したいのはシートの中の値なので、
// シートを開いたまま伝える。
function warnInSheet(msg) {
  const el = $('sheet-warn');
  el.textContent = msg;
  el.classList.remove('hidden');
}

function recordSet() {
  if (!sheetTarget) return;
  const { planned, recorded } = sheetTarget;

  const weight = parseFloat($('w').value);
  const reps = parseInt($('reps').value, 10);
  const rir = parseInt($('rir').value, 10);
  if (!Number.isFinite(weight) || !Number.isInteger(reps) || !Number.isInteger(rir)) {
    warnInSheet('重量・レップ・RIR を入れてください');
    return;
  }
  if (weight <= 0 || reps <= 0 || rir < 0) {
    warnInSheet('0 より大きい重量とレップを入れてください');
    return;
  }

  // 直す場合は、古い記録を消してから新しく入れる。同じIDで内容を
  // 変えるとサーバーが衝突として弾く（そういう契約にしてある）。
  if (recorded) {
    enqueue({ path: `/api/set-logs/${encodeURIComponent(recorded.id)}`, method: 'DELETE' });
  }

  // 直すときは同じIDを使い回す。新しいIDにすると、並び順が id 順
  // （＝作った時刻順）なので、直したセットだけが末尾に飛ぶ。
  // 1セット目を直したら3セット目になって出てくる。
  // 先に消してから入れ直すので、同一IDでも衝突にはならない。
  const id = recorded ? recorded.id : newId();
  enqueue({
    path: '/api/set-logs',
    body: { logs: [{ id, date: today(), exercise_id: planned.exercise_id, weight_kg: weight, reps, rir }] },
  });

  // 手元の表示も即座に更新する。送信の完了を待つと、
  // 電波が悪いときに「押したのに反応しない」画面になる。
  const list = state.doneToday.get(planned.exercise_id) || [];
  const next = recorded
    ? list.map((r) => (r.id === recorded.id ? { id, weight_kg: weight, reps, rir } : r))
    : [...list, { id, weight_kg: weight, reps, rir }];

  state.doneToday.set(planned.exercise_id, next);

  closeSheet();
  paintToday();
}

function undoSet() {
  if (!sheetTarget?.recorded) return;
  const { planned, recorded } = sheetTarget;
  enqueue({ path: `/api/set-logs/${encodeURIComponent(recorded.id)}`, method: 'DELETE' });

  state.doneToday.set(planned.exercise_id,
    (state.doneToday.get(planned.exercise_id) || []).filter((r) => r.id !== recorded.id));
  closeSheet();
  paintToday();
}

// --- 履歴 -----------------------------------------------------------

function sparkline(pts) {
  const w = 280, h = 44, pad = 4;
  // 2点では線が引けるだけで、推移として読めるものにならない。
  // 面まで塗ると、無い情報があるように見える。
  if (pts.length < 3) return '';
  const vals = pts.map((p) => p.kg);
  const min = Math.min(...vals), max = Math.max(...vals);
  const span = max - min || 1;
  const x = (i) => pad + (i * (w - pad * 2)) / (pts.length - 1);
  const y = (v) => h - pad - ((v - min) / span) * (h - pad * 2);
  const line = pts.map((p, i) => `${i ? 'L' : 'M'}${x(i).toFixed(1)},${y(p.kg).toFixed(1)}`).join(' ');
  return `<svg class="spark" viewBox="0 0 ${w} ${h}" preserveAspectRatio="none" aria-hidden="true">
    <path d="${line} L${x(pts.length - 1).toFixed(1)},${h} L${x(0).toFixed(1)},${h} Z" fill="rgba(224,168,46,.13)"></path>
    <path d="${line}" fill="none" stroke="#e0a82e" stroke-width="1.8" stroke-linecap="round"
      stroke-linejoin="round" vector-effect="non-scaling-stroke"></path>
    <circle cx="${x(pts.length - 1).toFixed(1)}" cy="${y(pts.at(-1).kg).toFixed(1)}" r="3" fill="#e0a82e"></circle>
  </svg>`;
}

let volumeExpanded = false;

// formatSets はその日のセットを1行にする。
//
// 全部同じ重量なら「100kg × 8, 8, 8」とまとめる。途中で重量を変えたら
// まとめられないので重量ごとに区切る。1セット目の重量で代表させると、
// 落とした重量も上げた重量も履歴から消える。
// formatLast は「前回」の1行。formatSets と同じ規則で畳む。
function formatLast(last) {
  const w = last.weights || [];
  return formatSets(last.reps.map((r, i) => ({ weight_kg: w[i] ?? last.weight_kg, reps: r })));
}

function formatSets(sets) {
  const groups = [];
  for (const s of sets) {
    const tail = groups[groups.length - 1];
    if (tail && tail.kg === s.weight_kg) tail.reps.push(s.reps);
    else groups.push({ kg: s.weight_kg, reps: [s.reps] });
  }
  return groups.map((g) => `${g.kg}kg × ${g.reps.join(', ')}`).join('　/　');
}

function paintHistory(stats, days) {
  // 21区分すべてを並べると長い。埋まっていない順に並んでいるので、
  // 上から数件だけ見えれば「次に何を足すか」は分かる。
  const all = stats.weekly_volume || [];
  const shown = volumeExpanded ? all : all.slice(0, 6);

  $('volume').innerHTML = shown.map((v) => {
    const pct = Math.min(100, Math.round((v.done_sets / Math.max(v.target_sets, .001)) * 100));
    return `<div class="vol-row">
        <span class="vol-name">${esc(regionName(v.region))}</span>
        <span class="vol-num">${v.done_sets.toFixed(1)} / ${v.target_sets.toFixed(1)}</span>
      </div>
      <div class="meter"><i class="${pct >= 100 ? 'full' : ''}" style="width:${pct}%"></i></div>`;
  }).join('') || '<p class="note">まだ記録がありません</p>';

  if (all.length > shown.length || volumeExpanded) {
    const more = el('button', 'chip', volumeExpanded
      ? '上位だけ表示' : `残り ${all.length - shown.length} 区分を表示`);
    more.style.marginTop = '10px';
    more.onclick = () => { volumeExpanded = !volumeExpanded; paintHistory(stats, days); };
    $('volume').append(more);
  }

  $('trend').innerHTML = (stats.trends || []).map((t) => {
    const sign = t.change_kg > 0 ? '+' : '';
    const cls = t.change_kg > 0 ? 'up' : t.change_kg < 0 ? 'down' : 'same';
    // 1回しか記録が無いときに「0.0」と出すと、伸びていないように読める。
    // 比べる相手がまだ無いだけなので、増減は出さない。
    const delta = (t.points || []).length < 2
      ? ''
      : `<span class="delta ${cls} num">${sign}${t.change_kg.toFixed(1)}</span>`;
    return `<div class="trend-row">
        <span class="trend-name">${esc(t.name)}</span>
        <span class="trend-val num">${t.current_kg.toFixed(1)}<small>kg</small>${delta}</span>
      </div>${sparkline(t.points)}`;
  }).join('') || '<p class="note">推移を出すには、同じ種目の記録が2回以上必要です</p>';

  $('days').innerHTML = (days || []).map((d) => `
    <div class="card">
      <div class="day-head">
        <span class="day-date">${label(d.date)}</span>
        <span class="day-meta">${d.exercises.length}種目 ${d.total_sets}セット</span>
      </div>
      <div class="log">
        ${d.exercises.map((e) => `
          <div class="log-row">
            <span>${esc(e.name || e.exercise_id)}</span>
            <span class="log-sets">${formatSets(e.sets)}</span>
          </div>`).join('')}
      </div>
    </div>`).join('') || '<div class="card"><p class="note">記録がまだありません</p></div>';
}

// --- 設定 -----------------------------------------------------------

function paintSettings() {
  $('per-week').value = String(state.program?.per_week ?? 3);

  $('pick').innerHTML = '';
  state.exercises
    .forEach((e) => {
      const b = el('button', 'chip', esc(e.name));
      b.setAttribute('aria-pressed', String(state.selected.has(e.id)));
      b.onclick = () => {
        state.selected.has(e.id) ? state.selected.delete(e.id) : state.selected.add(e.id);
        b.setAttribute('aria-pressed', String(state.selected.has(e.id)));
      };
      $('pick').append(b);
    });

  $('program-msg').textContent =
    'ここで選んだ種目だけが提案されます。';
}

async function saveProgram() {
  const body = {
    per_week: parseInt($('per-week').value, 10),
    weekly_target: state.program.weekly_target,
    selected_exercises: [...state.selected],
  };
  const res = await api('/api/program', { method: 'PUT', body: JSON.stringify(body) });
  if (res.ok) {
    $('program-msg').textContent = '保存しました';
    await loadAll();
    return;
  }
  const err = await res.json().catch(() => ({}));
  $('program-msg').textContent = err.error || `保存に失敗しました (${res.status})`;
}

// --- 読み込み -------------------------------------------------------

function showSetup(message) {
  showView('setup');
  $('tabs').classList.add('hidden');
  $('setup-error').textContent = message || '';

  // 溜まっているものは消えない。ここで言わないと、記録ごと消えたと
  // 思われる。トークンが変わったのは送り先の話で、記録の話ではない。
  const n = pendingCount;
  const pending = $('setup-pending');
  pending.textContent = n > 0
    ? `未送信の記録が ${n} 件あります。トークンを入れ直せば送られます。` : '';
  pending.classList.toggle('hidden', n === 0);
}

function showView(name) {
  state.view = name;
  ['setup', 'today', 'history', 'settings'].forEach((v) => { $(v).hidden = v !== name; });
  document.querySelectorAll('.tab').forEach((t) =>
    t.setAttribute('aria-selected', String(t.dataset.view === name)));
  window.scrollTo(0, 0);
}

async function loadToday(deloadAccepted) {
  // 引数が無いときは、その日に承認したものを使う。更新やリロードでも
  // 承認が効いたままになる。
  const accepted = deloadAccepted ?? store.deload(today());
  const q = accepted?.length
    ? `&deload_accepted=${encodeURIComponent(accepted.join(','))}` : '';
  const res = await api(`/api/sessions?date=${today()}${q}`);
  if (!res.ok) throw new Error(`sessions ${res.status}`);
  state.session = await res.json();
  paintToday();
}

async function loadAll() {
  if (!store.token()) return showSetup();
  $('when').textContent = label(today());
  $('tabs').classList.remove('hidden');

  // 溜まっているものを先に送りきってから読む。
  //
  // 逆にすると、送信前の状態で描画してから送ることになり、記録したのに
  // 緑が消えて見える。オフラインで記録して復帰したときに必ず踏み、
  // 「消えた」と思ってもう一度記録して重複する。
  await flush();

  try {
    const [exRes, logRes, statRes, progRes] = await Promise.all([
      api('/api/exercises'),
      api(`/api/set-logs?from=${addDays(today(), -56)}&to=${today()}`),
      api(`/api/stats?from=${addDays(today(), -180)}&to=${today()}`),
      api('/api/program'),
    ]);

    if (exRes.ok) {
      const { exercises } = await exRes.json();
      state.exercises = exercises;
      state.names = new Map(exercises.map((e) => [e.id, e.name]));
    }

    if (logRes.ok) {
      const body = await logRes.json();
      state.last = body.last_performances || {};
      state.days = body.days || [];

      // 「どこまでやったか」をサーバーの実績から復元する。
      // 端末の中だけに持つと、画面を閉じた瞬間に分からなくなる。
      state.doneToday = new Map();
      const t = (body.days || []).find((d) => d.date === today());
      (t?.exercises || []).forEach((e) => {
        state.doneToday.set(e.exercise_id, e.sets.map((s) => ({
          id: s.id, weight_kg: s.weight_kg, reps: s.reps, rir: s.rir,
        })));
      });
    }

    state.stats = statRes.ok ? await statRes.json() : { trends: [], weekly_volume: [] };

    if (progRes.ok) {
      state.program = await progRes.json();
      state.selected = new Set(state.program.selected_exercises || []);
    }

    await loadToday();
    paintHistory(state.stats, state.days);
    paintSettings();

    // 初期状態は全ての画面が hidden なので、必ずどれかに切り替える。
    // 「setup が隠れているか」で判定すると、初回に何も表示されない。
    $('offline-notice').classList.add('hidden');
    paintRejected();
    showView(state.view && state.view !== 'setup' ? state.view : 'today');
    paintStatus();
  } catch (e) {
    if (String(e.message) === 'unauthorized') return;
    $('dot').className = 'dot error';
    $('status-text').textContent = 'つながりません';

    // ここで画面を切り替えないと、圏外で開いたときに何も出ない。
    // 起動直後は全ての画面が hidden なので、状態表示だけの白い画面になる。
    $('offline-notice').classList.remove('hidden');
    showView(state.view && state.view !== 'setup' ? state.view : 'today');
  }
}

// --- 配線 -----------------------------------------------------------

$('save-token').onclick = () => {
  const v = $('token').value.trim();
  if (v.length < 32) { $('setup-error').textContent = 'トークンが短すぎます'; return; }
  store.setToken(v);
  $('token').value = '';
  showView('today');
  loadAll();
};

$('forget').onclick = () => {
  if (!confirm('この端末からトークンを消します。よろしいですか')) return;
  store.clearToken();
  showSetup();
};

$('record').onclick = recordSet;
$('undo').onclick = undoSet;
$('sheet-close').onclick = closeSheet;
$('reload').onclick = () => loadAll();
$('retry').onclick = () => loadAll();
$('clear-rejected').onclick = () =>
  outbox.clear('rejected').then(refreshOutbox).then(paintRejected);
$('save-program').onclick = saveProgram;

function noteCondition(msg) {
  const el = $('condition-note');
  el.textContent = msg;
  el.classList.toggle('hidden', !msg);
}

$('save-condition').onclick = () => {
  const bw = parseFloat($('bw').value);
  const sl = parseFloat($('sl').value);
  const item = { date: today() };
  if (Number.isFinite(bw)) item.body_weight_kg = bw;
  if (Number.isFinite(sl)) item.sleep_hours = sl;
  if (!('body_weight_kg' in item) && !('sleep_hours' in item)) {
    noteCondition('体重か睡眠のどちらかを入れてください');
    return;
  }
  noteCondition('');
  enqueue({ path: '/api/conditions', body: { conditions: [item] } });
  $('bw').value = '';
  $('sl').value = '';
};

document.querySelectorAll('[data-t]').forEach((b) => {
  b.onclick = () => {
    const input = $(b.dataset.t);
    const now = parseFloat(input.value);
    const next = (Number.isFinite(now) ? now : 0) + parseFloat(b.dataset.by);
    input.value = Math.max(0, Math.round(next * 100) / 100);
  };
});

document.querySelectorAll('.tab').forEach((t) => {
  t.onclick = () => showView(t.dataset.view);
});

window.addEventListener('online', () => { paintStatus(); flush(); });
window.addEventListener('offline', paintStatus);

if ('serviceWorker' in navigator) {
  navigator.serviceWorker.register('/sw.js').catch(() => {});
}

// 待ち行列を読み込んでから始める。先に loadAll を走らせると、
// 溜まっているぶんを送る前にサーバーの実績で画面を描いてしまう。
adoptLegacyQueue()
  .then(refreshOutbox)
  .catch(() => {})
  .then(() => loadAll());
