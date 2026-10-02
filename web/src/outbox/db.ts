// 送信の待ち行列を IndexedDB に置く。
//
// localStorage には「読んで、足して、書く」を割り込まれずに行う手段が
// 無い。携帯とPCのように同じ端末で2つ開いていると、両方が同じキーを
// 読んで書き戻し、あとから書いた方が相手の記録を消す。実際に2タブから
// 3セットずつ記録したら、6件のうち3件が消えた。
//
// IndexedDB の書き込みはトランザクションなので、追記どうしがぶつからない。
// 2つのタブが同時に送っても、二重に送るだけで済む（同じIDの同じ内容は
// サーバーが黙って受け入れる契約にしてある）。消えるよりはるかによい。
//
// DB 名・版・ストア名は旧版（html + js）から変えない。変えると、
// 送りきれていない記録が読めなくなる。

const DB_NAME = 'liftplan';
const DB_VERSION = 1;

export type StoreName = 'queue' | 'rejected';

/** QueueItem は1件の送信要求。ID はクライアントが採番済み。 */
export type QueueItem = {
  path: string;
  method?: string;
  body?: unknown;
};

/** Enqueue は待ち行列に積む関数。端末に積めたら true、積めなかったら false を返す。
 *
 *  送信の完了は待たない。積めなかったときに黙って返すと、呼び手は保存できた
 *  つもりで画面を進めてしまうので、成否を戻り値で渡す。 */
export type Enqueue = (item: QueueItem) => Promise<boolean>;

/** Entry は待ち行列の1件と、その自動採番キー。 */
export type Entry<T> = { key: IDBValidKey; value: T };

function openDB(): Promise<IDBDatabase> {
  return new Promise((resolve, reject) => {
    const req = indexedDB.open(DB_NAME, DB_VERSION);
    req.onupgradeneeded = () => {
      const db = req.result;
      for (const name of ['queue', 'rejected'] satisfies StoreName[]) {
        if (!db.objectStoreNames.contains(name)) {
          db.createObjectStore(name, { autoIncrement: true });
        }
      }
    };
    req.onsuccess = () => resolve(req.result);
    req.onerror = () => reject(req.error);
  });
}

let dbPromise: Promise<IDBDatabase> | null = null;

function db(): Promise<IDBDatabase> {
  dbPromise ??= openDB();
  return dbPromise;
}

function tx<T>(
  name: StoreName,
  mode: IDBTransactionMode,
  run: (store: IDBObjectStore) => IDBRequest<T> | undefined,
): Promise<T | undefined> {
  return db().then(
    (d) =>
      new Promise<T | undefined>((resolve, reject) => {
        const t = d.transaction(name, mode);
        let req: IDBRequest<T> | undefined;
        try {
          req = run(t.objectStore(name));
        } catch (e) {
          // 積めない値だった。ここまでに積んだぶんが残ったまま確定しないよう、
          // トランザクションごと取り消す（addAll が「全部か、1件も」を守るため）。
          try {
            t.abort();
          } catch {
            // すでに終わっている。
          }
          reject(e);
          return;
        }
        // 完了を待ってから解決する。req.onsuccess で返すと、
        // トランザクションが中断された書き込みを成功として扱う。
        t.oncomplete = () => resolve(req ? req.result : undefined);
        t.onerror = () => reject(t.error);
        t.onabort = () => reject(t.error);
      }),
  );
}

export const add = <T>(name: StoreName, value: T): Promise<void> =>
  tx<IDBValidKey>(name, 'readwrite', (s) => s.add(value)).then(() => undefined);

/** addAll は複数件を1つのトランザクションで積む。全部積めるか、1件も積まれないか。
 *
 *  1件ずつ別のトランザクションで積むと、途中で止まったときに前半だけが
 *  残る。修正は「DELETE を積んでから同じIDで POST」の2件で表すので、
 *  DELETE だけが残ると、直したつもりの記録が消える。順は渡した順のまま
 *  （自動採番キーが増える順）。 */
export const addAll = <T>(name: StoreName, items: readonly T[]): Promise<void> => {
  return tx<IDBValidKey>(name, 'readwrite', (s) => {
    let last: IDBRequest<IDBValidKey> | undefined;
    for (const item of items) last = s.add(item);
    return last;
  }).then(() => undefined);
};

export const remove = (name: StoreName, key: IDBValidKey): Promise<void> =>
  tx<undefined>(name, 'readwrite', (s) => s.delete(key) as IDBRequest<undefined>).then(() => undefined);

export const clear = (name: StoreName): Promise<void> =>
  tx<undefined>(name, 'readwrite', (s) => s.clear() as IDBRequest<undefined>).then(() => undefined);

export const count = (name: StoreName): Promise<number> =>
  tx<number>(name, 'readonly', (s) => s.count()).then((n) => n ?? 0);

export const values = <T>(name: StoreName): Promise<T[]> =>
  tx<T[]>(name, 'readonly', (s) => s.getAll() as IDBRequest<T[]>).then((v) => v ?? []);

/** head は待ち行列の先頭を1件だけ読む。全件読むと、溜まったときに重い。 */
export function head<T>(name: StoreName): Promise<Entry<T> | null> {
  return db().then(
    (d) =>
      new Promise<Entry<T> | null>((resolve, reject) => {
        const t = d.transaction(name, 'readonly');
        const cur = t.objectStore(name).openCursor();
        cur.onsuccess = () =>
          resolve(cur.result ? { key: cur.result.key, value: cur.result.value as T } : null);
        cur.onerror = () => reject(cur.error);
      }),
  );
}

/** clearAll はテスト用。両方のストアを空にする。 */
export async function clearAll(): Promise<void> {
  await clear('queue');
  await clear('rejected');
}

// 旧版が localStorage に残したぶんを引き取る。捨てると記録が消える。
const LEGACY: [string, StoreName][] = [
  ['liftplan.queue', 'queue'],
  ['liftplan.rejected', 'rejected'],
];

export async function adoptLegacyQueue(): Promise<void> {
  for (const [key, name] of LEGACY) {
    let items: unknown[];
    try {
      items = JSON.parse(localStorage.getItem(key) ?? '[]') as unknown[];
    } catch {
      items = [];
    }
    if (!Array.isArray(items) || items.length === 0) continue;
    for (const it of items) await add(name, it);
    localStorage.removeItem(key);
  }
}
