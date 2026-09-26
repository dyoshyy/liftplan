import { useCallback, useEffect, useState } from 'react';
import { send } from '../api/client';
import {
  buildQuery,
  defaultForm,
  describeFailure,
  withFocusInDeclared,
  type DevOptions,
  type DevResult,
  type Form,
} from './simulate';

// 口は認証の内側にあるので api/client.ts の send を通す。あちらが
// トークンを載せ、401 なら捨てて Unauthorized を投げる。
//
// client.getJSON は使わない。あちらは状態コードしか throw せず、
// サーバーが本文に書いた 400 の理由が消える。ここではその理由こそが
// 見たいもの（どの設定が成り立たないか）なので、本文を読む。
async function getJSON<T>(path: string): Promise<T> {
  const res = await send({ path });
  if (!res.ok) {
    const body = (await res.json().catch(() => null)) as { error?: string } | null;
    throw new Error(body?.error ?? `${path} が ${res.status} を返した`);
  }
  return (await res.json()) as T;
}

/** useSimulation は設定を持ち、サーバーに計画を作らせる。
 *
 *  判断は simulate.ts に置いてある。ここは順序と状態だけ。 */
export function useSimulation() {
  const [options, setOptions] = useState<DevOptions | null>(null);
  const [form, setFormState] = useState<Form>(defaultForm);
  const [result, setResult] = useState<DevResult | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  // 設定を変えたら、重点種目が宣言から外れていないかを必ず通す。
  const setForm = useCallback((next: Form) => setFormState(withFocusInDeclared(next)), []);

  const run = useCallback(async (target: Form) => {
    setBusy(true);
    setError(null);
    try {
      setResult(await getJSON<DevResult>(`/api/dev/simulate?${buildQuery(target)}`));
    } catch (e) {
      setResult(null);
      setError(describeFailure(e));
    } finally {
      setBusy(false);
    }
  }, []);

  // 開いた時点で既定の設定の結果を出す。空の画面から始めると、
  // 何が見られる道具なのかが分からない。
  useEffect(() => {
    let alive = true;
    void (async () => {
      try {
        const got = await getJSON<DevOptions>('/api/dev/options');
        if (alive) setOptions(got);
      } catch (e) {
        if (alive) setError(describeFailure(e));
      }
    })();
    void run(defaultForm);
    return () => {
      alive = false;
    };
  }, [run]);

  return { options, form, setForm, result, error, busy, run: () => void run(form) };
}
