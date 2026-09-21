import { useCallback, useEffect, useState } from 'react';
import {
  buildQuery,
  defaultForm,
  withFocusInDeclared,
  type DevOptions,
  type DevResult,
  type Form,
} from './simulate';

// 開発用の口は認証の外に置いてある（捏造した設定で計画を作るだけで、
// 保存先も利用者の記録も触らない）。だから api/client.ts は通さない。
// あちらはトークンと待ち行列の都合を持っていて、ここには要らない。
const API_BASE = import.meta.env.VITE_API_BASE;

async function getJSON<T>(path: string): Promise<T> {
  const res = await fetch(API_BASE + path);
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
      setError(e instanceof Error ? e.message : String(e));
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
        if (alive) setError(e instanceof Error ? e.message : String(e));
      }
    })();
    void run(defaultForm);
    return () => {
      alive = false;
    };
  }, [run]);

  return { options, form, setForm, result, error, busy, run: () => void run(form) };
}
