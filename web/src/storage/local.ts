// localStorage に置くのは、失っても記録が消えないものだけ。
//
// 記録そのものは IndexedDB の待ち行列に置く。localStorage には
// 「読んで、足して、書く」を割り込まれずに行う手段が無いため（outbox/db.ts）。

const KEY_TOKEN = 'liftplan.token';
const KEY_DELOAD = 'liftplan.deload';

export const getToken = (): string => localStorage.getItem(KEY_TOKEN) ?? '';
export const setToken = (v: string): void => localStorage.setItem(KEY_TOKEN, v);
export const clearToken = (): void => localStorage.removeItem(KEY_TOKEN);

// 承認したデロードはその日のあいだ覚えておく。覚えないと、更新した
// とたんに指示が10%跳ね上がる。日付ごとに持つので翌日には消える。
export function acceptedDeload(date: string): string[] {
  try {
    const v = JSON.parse(localStorage.getItem(KEY_DELOAD) ?? 'null') as
      | { date?: string; ids?: string[] }
      | null;
    return v && v.date === date ? (v.ids ?? []) : [];
  } catch {
    return [];
  }
}

export function setAcceptedDeload(date: string, ids: string[]): void {
  localStorage.setItem(KEY_DELOAD, JSON.stringify({ date, ids }));
}
