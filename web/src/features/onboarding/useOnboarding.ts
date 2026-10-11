import { useEffect, useState } from 'react';
import { getJSON, send } from '../../api/client';
import type { Program, SplitPreset, SplitPresetsResponse } from '../../api/types';
import { describePutFailure } from '../settings/useProgramSettings';
import { draftFrom, planOnboardingRequests, STEPS, type Draft } from './onboarding';

// 初回の設定の手順。判断は onboarding.ts に置き、ここは順に実行するだけ。
//
// 設定画面と同じく待ち行列を通さずその場で送る。途中の段階では送らない。
// 閉じたときに半端な設定が残り、しかも「済んだ」にならないため。
export function useOnboarding(onDone: () => Promise<void>) {
  const [program, setProgram] = useState<Program | null>(null);
  const [presets, setPresets] = useState<SplitPreset[]>([]);
  const [draft, setDraft] = useState<Draft | null>(null);
  const [step, setStep] = useState(0);
  const [note, setNote] = useState('');
  const [busy, setBusy] = useState(false);

  const load = async () => {
    setNote('');
    try {
      const [p, sp] = await Promise.all([
        getJSON<Program>('/api/program'),
        getJSON<SplitPresetsResponse>('/api/split-presets'),
      ]);
      setProgram(p);
      setPresets(sp.presets);
      setDraft(draftFrom(p, sp.presets));
    } catch {
      setNote('設定を読めませんでした');
    }
  };

  useEffect(() => {
    void load();
    // 初回だけ読む。load は描画ごとに作り直される（useProgramSettings と同じ）。
  }, []);

  const next = () => setStep((s) => Math.min(s + 1, STEPS.length - 1));
  const back = () => setStep((s) => Math.max(s - 1, 0));

  // finish は「はじめる」で全部を順に送る。1件でも断られたら止め、理由を出す。
  // 送り直すときは最初から送る（どれも冪等）。
  const finish = async () => {
    if (!program || !draft || busy) return;
    if (!navigator.onLine) {
      setNote('つながらないので変えられません');
      return;
    }
    setBusy(true);
    setNote('');
    let failed = true;
    try {
      for (const req of planOnboardingRequests(program, draft)) {
        const res = await send({ path: req.path, method: 'PUT', body: req.body });
        if (!res.ok) {
          const failure = (await res.json().catch(() => null)) as { error?: string } | null;
          setNote(describePutFailure(res.status, failure));
          return;
        }
      }
      failed = false;
      await onDone();
    } catch {
      setNote('つながらないので変えられません');
    } finally {
      // 途中まで通っていれば、サーバーの値は読んだときと違う。送り直しの
      // 順序は今の値で決まる（planOnboardingRequests）ので、読み直しておく。
      // 下書きは触らない。選び直した内容が消える。
      if (failed) {
        await getJSON<Program>('/api/program')
          .then(setProgram)
          .catch(() => undefined);
      }
      setBusy(false);
    }
  };

  return { program, presets, draft, step, note, busy, setDraft, next, back, finish };
}
