// localStorage に置くのは、失っても記録が消えないものだけ。
//
// 記録そのものは IndexedDB の待ち行列に置く。localStorage には
// 「読んで、足して、書く」を割り込まれずに行う手段が無いため（outbox/db.ts）。
//
// キー名は旧版から変えない。変えると、送りきれていない記録が読めなくなる。

const KEY_TOKEN = 'liftplan.token';

export const getToken = (): string => localStorage.getItem(KEY_TOKEN) ?? '';
export const setToken = (v: string): void => localStorage.setItem(KEY_TOKEN, v);
export const clearToken = (): void => localStorage.removeItem(KEY_TOKEN);

// --- 休憩タイマー ---
//
// 長さは本人が決めたもの。状態（動いているか・いつ始めたか）も残す。
// 残さないと、画面を閉じて開き直したときにタイマーが消える。ジムでは
// セットの合間に画面を消すので、毎回消えると使い物にならない。

const KEY_REST_DURATION = 'liftplan.rest.duration';
const KEY_REST_STATE = 'liftplan.rest.state';
const KEY_REST_VOLUME = 'liftplan.rest.volume';

export function readJSON<T>(key: string): T | null {
  try {
    const raw = localStorage.getItem(key);
    return raw === null ? null : (JSON.parse(raw) as T);
  } catch {
    // 壊れた値が入っていても、そこで画面ごと止めない。既定に戻す。
    return null;
  }
}

export function writeJSON(key: string, value: unknown): void {
  try {
    localStorage.setItem(key, JSON.stringify(value));
  } catch {
    // 容量超過やプライベートモード。タイマーが残らないだけで、記録には影響しない。
  }
}

export const restKeys = {
  duration: KEY_REST_DURATION,
  state: KEY_REST_STATE,
  volume: KEY_REST_VOLUME,
} as const;
