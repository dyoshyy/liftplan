'use strict';

// liftplan の最小クライアント。
//
// 設計の要点は3つ。
//
// 1. 記録を失わない。ジムの電波は途切れる。送信は待ち行列に積んでから
//    投げ、失敗しても消さない。サーバーは同じIDの同じ内容を黙って
//    受け入れるので（冪等）、何度でも再送してよい。
// 2. IDはクライアントが採番する。サーバーが採番すると、応答が届かなかった
//    ときに「保存されたのか分からない」が発生する。
// 3. 未来のメニューを持たない。表示は常にサーバーから取り直す。
//    手元で持つと、記録した結果が反映されているのか分からなくなる。

const KEY_TOKEN = 'liftplan.token';
const KEY_QUEUE = 'liftplan.queue';

const $ = (id) => document.getElementById(id);

/** 今日の日付を YYYY-MM-DD で返す。端末のローカル日付で扱う。 */
function today() {
  const d = new Date();
  const p = (n) => String(n).padStart(2, '0');
  return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())}`;
}

/** 衝突しないIDを作る。時刻順に並ぶので、後から見て追いやすい。 */
function newId() {
  const t = Date.now().toString(36);
  const r = Math.random().toString(36).slice(2, 10);
  return `w-${t}-${r}`;
}

const store = {
  token: () => localStorage.getItem(KEY_TOKEN) || '',
  setToken: (v) => localStorage.setItem(KEY_TOKEN, v),
  clearToken: () => localStorage.removeItem(KEY_TOKEN),
  queue: () => JSON.parse(localStorage.getItem(KEY_QUEUE) || '[]'),
  setQueue: (q) => localStorage.setItem(KEY_QUEUE, JSON.stringify(q)),
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

// --- 送信の待ち行列 ---------------------------------------------------

/** 送るものを積む。まず保存し、それから送る。順序が逆だと失敗時に消える。 */
function enqueue(item) {
  const q = store.queue();
  q.push(item);
  store.setQueue(q);
  render.status();
  flush();
}

let flushing = false;

/** 溜まったものを送る。送れたものだけ消す。 */
async function flush() {
  if (flushing || !store.token()) return;
  flushing = true;
  try {
    let q = store.queue();
    while (q.length > 0) {
      const item = q[0];
      const res = await api(item.path, { method: 'POST', body: JSON.stringify(item.body) });

      if (res.ok) {
        q = q.slice(1);
        store.setQueue(q);
        continue;
      }
      if (res.status === 409) {
        // 同じIDで内容が違う。再送しても永久に通らないので捨てる。
        // 残すと後続が全部詰まる。
        console.warn('衝突のため破棄', item);
        q = q.slice(1);
        store.setQueue(q);
        continue;
      }
      if (res.status >= 400 && res.status < 500) {
        // 入力が不正。送り直しても通らない。
        console.warn('不正なので破棄', item, await res.text());
        q = q.slice(1);
        store.setQueue(q);
        continue;
      }
      // 5xx / 503 はやり直せば通る。残したまま抜ける。
      break;
    }
  } catch (e) {
    // 電波が無い。次の機会に送る。
  } finally {
    flushing = false;
    render.status();
  }
}

// --- 画面 -------------------------------------------------------------

let session = null;
const done = new Map(); // "exerciseID#index" -> true

const render = {
  status() {
    const pending = store.queue().length;
    const dot = $('dot');
    dot.className = 'dot' + (pending > 0 ? ' pending' : '');
    $('status-text').textContent = pending > 0
      ? `未送信 ${pending} 件`
      : (navigator.onLine ? '同期済み' : 'オフライン（記録は保存されます）');
    if (!navigator.onLine) dot.className = 'dot error';
  },

  session() {
    const root = $('session');
    root.innerHTML = '';
    if (!session) return;

    const group = (title, sets) => {
      if (sets.length === 0) return;
      const h = document.createElement('p');
      h.className = 'small muted';
      h.style.margin = '18px 4px 8px';
      h.textContent = title;
      root.appendChild(h);
      sets.forEach((s) => root.appendChild(exerciseCard(s)));
    };

    group('メイン', session.main);
    group('補助', session.accessories);
  },
};

function exerciseCard(planned) {
  const card = document.createElement('div');
  card.className = 'card';

  const head = document.createElement('div');
  head.className = 'ex-head';
  const name = document.createElement('span');
  name.className = 'ex-name';
  name.textContent = planned.exercise_id;
  head.appendChild(name);
  if (planned.role) {
    const badge = document.createElement('span');
    badge.className = 'badge';
    badge.textContent = planned.role;
    head.appendChild(badge);
  }
  card.appendChild(head);

  const p = document.createElement('div');
  p.className = 'prescription';
  const weight = planned.weight_kg === null
    ? '<strong>未確定</strong>'
    : `<strong>${planned.weight_kg}</strong> kg`;
  p.innerHTML = `${weight} × ${planned.sets}セット　目標RIR ${planned.target_rir}`;
  card.appendChild(p);

  if (planned.weight_kg === null) {
    const note = document.createElement('p');
    note.className = 'small muted';
    note.style.marginTop = '-6px';
    note.textContent = '記録がまだ足りません。初回は自分で決めて入れてください。';
    card.appendChild(note);
  }

  const sets = document.createElement('div');
  sets.className = 'sets';
  for (let i = 0; i < planned.sets; i++) {
    const key = `${planned.exercise_id}#${i}`;
    const b = document.createElement('button');
    b.className = 'set' + (done.has(key) ? ' done' : '');
    const n = document.createElement('span');
    n.className = 'n';
    n.textContent = `${i + 1}セット目`;
    const v = document.createElement('span');
    v.className = 'v';
    v.textContent = done.has(key) ? done.get(key) : '記録';
    b.append(n, v);
    b.addEventListener('click', () => openSheet(planned, i, key));
    sets.appendChild(b);
  }
  card.appendChild(sets);
  return card;
}

// --- 記録のシート -----------------------------------------------------

let sheetTarget = null;

function openSheet(planned, index, key) {
  sheetTarget = { planned, index, key };
  $('sheet-title').textContent = `${planned.exercise_id} ${index + 1}セット目`;
  $('w').value = planned.weight_kg === null ? '' : planned.weight_kg;
  $('reps').value = 8;
  $('rir').value = planned.target_rir;
  $('sheet').showModal();
}

function closeSheet() {
  $('sheet').close();
  sheetTarget = null;
}

function recordSet() {
  if (!sheetTarget) return;
  const { planned, key } = sheetTarget;

  const weight = parseFloat($('w').value);
  const reps = parseInt($('reps').value, 10);
  const rir = parseInt($('rir').value, 10);
  if (!Number.isFinite(weight) || !Number.isInteger(reps) || !Number.isInteger(rir)) {
    alert('重量・レップ・RIR を入れてください');
    return;
  }

  enqueue({
    path: '/api/set-logs',
    body: {
      logs: [{
        id: newId(),
        date: today(),
        exercise_id: planned.exercise_id,
        weight_kg: weight,
        reps,
        rir,
      }],
    },
  });

  done.set(key, `${weight}×${reps}`);
  closeSheet();
  render.session();
}

// --- 読み込み ---------------------------------------------------------

function showSetup(message) {
  $('setup').classList.remove('hidden');
  $('app').classList.add('hidden');
  $('setup-error').textContent = message || '';
}

async function load(deloadAccepted) {
  if (!store.token()) return showSetup();

  $('date').textContent = today();
  const query = deloadAccepted && deloadAccepted.length > 0
    ? `&deload_accepted=${encodeURIComponent(deloadAccepted.join(','))}`
    : '';

  try {
    const res = await api(`/api/sessions?date=${today()}${query}`);
    if (!res.ok) {
      $('status-text').textContent = `取得に失敗 (${res.status})`;
      $('dot').className = 'dot error';
      return;
    }
    session = await res.json();
  } catch (e) {
    $('status-text').textContent = 'つながりません';
    $('dot').className = 'dot error';
    return;
  }

  $('setup').classList.add('hidden');
  $('app').classList.remove('hidden');

  const d = session.deload_proposal;
  if (d && d.stalled_exercises && d.stalled_exercises.length > 0) {
    $('deload').classList.remove('hidden');
    $('deload-reason').textContent = d.reason;
    $('accept-deload').onclick = () => load(d.stalled_exercises);
  } else {
    $('deload').classList.add('hidden');
  }

  render.session();
  render.status();
  flush();
}

// --- 配線 -------------------------------------------------------------

$('save-token').addEventListener('click', () => {
  const v = $('token').value.trim();
  if (v.length < 32) {
    $('setup-error').textContent = 'トークンが短すぎます';
    return;
  }
  store.setToken(v);
  $('token').value = '';
  load();
});

$('sheet-close').addEventListener('click', closeSheet);
$('record').addEventListener('click', recordSet);
$('reload').addEventListener('click', () => load());

document.querySelectorAll('[data-step]').forEach((b) => {
  b.addEventListener('click', () => {
    const input = $(b.dataset.step);
    const by = parseFloat(b.dataset.by);
    const now = parseFloat(input.value);
    const next = (Number.isFinite(now) ? now : 0) + by;
    input.value = Math.max(0, Math.round(next * 100) / 100);
  });
});

$('save-condition').addEventListener('click', () => {
  const bw = parseFloat($('bw').value);
  const sleep = parseFloat($('sleep').value);
  const item = { date: today() };
  if (Number.isFinite(bw)) item.body_weight_kg = bw;
  if (Number.isFinite(sleep)) item.sleep_hours = sleep;
  if (!('body_weight_kg' in item) && !('sleep_hours' in item)) {
    alert('体重か睡眠のどちらかを入れてください');
    return;
  }
  enqueue({ path: '/api/conditions', body: { conditions: [item] } });
  $('bw').value = '';
  $('sleep').value = '';
});

window.addEventListener('online', () => { render.status(); flush(); });
window.addEventListener('offline', render.status);

if ('serviceWorker' in navigator) {
  navigator.serviceWorker.register('/sw.js').catch(() => {});
}

load();
