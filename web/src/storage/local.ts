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
