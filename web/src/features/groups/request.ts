import { send } from '../../api/client';
import { describeFailure } from './groups';

/** Sent は送った結果。ok でなければ画面に出す一言を持つ。 */
export type Sent<T> = { ok: true; value: T } | { ok: false; note: string };

/**
 * sendGroupRequest はグループの書き込みをその場で送る（待ち行列に乗せない）。
 * 断られたらサーバーの理由を、届かなければその旨を返す。
 */
export async function sendGroupRequest<T>(path: string, method: string, body?: unknown): Promise<Sent<T>> {
  try {
    const res = await send({ path, method, body });
    if (!res.ok) {
      const failure = (await res.json().catch(() => null)) as { error?: string } | null;
      return { ok: false, note: describeFailure(res.status, failure) };
    }
    return { ok: true, value: res.status === 204 ? ({} as T) : ((await res.json()) as T) };
  } catch {
    return { ok: false, note: 'つながらないのでできません' };
  }
}
