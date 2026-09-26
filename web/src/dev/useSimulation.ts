import { useCallback, useEffect, useState } from 'react';
import { send } from '../api/client';
import {
  buildQuery,
  defaultForm,
  describeFailure,
  parseForm,
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

// 開いたときの設定は URL から読む。URL 1本で同じ状況を再現できるように
// する（Claude が URL を開いて結果を読みに来る）。
const initialForm = (): Form => withFocusInDeclared(parseForm(window.location.search, defaultForm));

/** useSimulation は設定を持ち、サーバーに計画を作らせる。
 *
 *  判断は simulate.ts に置いてある。ここは順序と状態だけ。 */
export function useSimulation() {
  const [options, setOptions] = useState<DevOptions | null>(null);
  const [form, setFormState] = useState<Form>(initialForm);
  const [result, setResult] = useState<DevResult | null>(null);
  // 結果を作ったときの設定。フォームは「作る」を押す前に変えられるので、
  // グラフが宣言種目を引くのは、いまのフォームではなくこちら。
  const [ranForm, setRanForm] = useState<Form | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  // 設定を変えたら、重点種目が宣言から外れていないかを必ず通す。
  const setForm = useCallback((next: Form) => setFormState(withFocusInDeclared(next)), []);

  const run = useCallback(async (target: Form) => {
    setBusy(true);
    setError(null);
    try {
      const query = buildQuery(target);
      setResult(await getJSON<DevResult>(`/api/dev/simulate?${query}`));
      setRanForm(target);
      // 結果を出した設定を URL に残す。開き直しても、共有しても同じ結果になる。
      window.history.replaceState(null, '', `?${query}`);
    } catch (e) {
      setResult(null);
      setRanForm(null);
      setError(describeFailure(e));
    } finally {
      setBusy(false);
    }
  }, []);

  // 開いた時点で URL（無ければ既定）の設定の結果を出す。空の画面から
  // 始めると、何が見られる道具なのかが分からない。
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
    void run(initialForm());
    return () => {
      alive = false;
    };
  }, [run]);

  return { options, form, setForm, result, ranForm, error, busy, run: () => void run(form) };
}
