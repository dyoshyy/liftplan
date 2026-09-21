import { getToken, clearToken } from '../storage/local';

// API_BASE は別オリジンのサーバー。開発でも本番と同じくクロスオリジンで叩く。
//
// 開発で proxy を挟んで同一オリジンに見せると、CORS を一度も通らないまま
// 開発が終わり、設定漏れが本番で初めて出る。
//
// **空なら即座に落とす。**同一オリジンに落とすと、送り先が画面を配っている
// Worker になる。あちらは未知の経路に index.html を 200 で返すので、
// 待ち行列は「送れた」と判断して記録を消す。記録が黙って消える経路になる。
// サーバー側が ALLOWED_ORIGINS 無しで起動を拒むのと同じ理由。
export const API_BASE: string = import.meta.env.VITE_API_BASE;
if (!API_BASE) {
  throw new Error('VITE_API_BASE が設定されていない。API のオリジンを指定すること');
}

/** Unauthorized はトークンが通らなかったことを表す。画面はこれを見て設定へ戻す。 */
export class Unauthorized extends Error {
  constructor() {
    super('unauthorized');
    this.name = 'Unauthorized';
  }
}

export type Request = {
  path: string;
  method?: string;
  /** JSON にして送る本体。undefined なら Content-Type を付けない。 */
  body?: unknown;
};

// send は1回の HTTP を投げる。応答の解釈はしない。
//
// 待ち行列は 4xx と 5xx で扱いを変えるので、ここで throw に潰さない。
// Response をそのまま返し、判断は呼び手に残す。
export async function send(req: Request): Promise<Response> {
  const res = await fetch(API_BASE + req.path, {
    method: req.method ?? (req.body === undefined ? 'GET' : 'POST'),
    headers: {
      Authorization: `Bearer ${getToken()}`,
      ...(req.body === undefined ? {} : { 'Content-Type': 'application/json' }),
    },
    body: req.body === undefined ? undefined : JSON.stringify(req.body),
  });

  if (res.status === 401) {
    clearToken();
    throw new Unauthorized();
  }
  return res;
}

// getJSON は読み取り用。失敗は例外にする。
export async function getJSON<T>(path: string): Promise<T> {
  const res = await send({ path });
  if (!res.ok) throw new Error(`${path} が ${res.status} を返した`);
  return (await res.json()) as T;
}
