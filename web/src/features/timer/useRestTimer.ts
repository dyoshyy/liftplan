import { useCallback, useEffect, useRef, useState } from 'react';
import { readJSON, restKeys, writeJSON } from '../../storage/local';
import { askNotificationPermission, beep, notifyRestOver, primeSound } from './alert';
import { clampVolume, DEFAULT_VOLUME } from './volume';
import {
  clampDuration,
  DEFAULT_DURATION_SEC,
  remainingMs,
  type RestState,
} from './rest';

/** TICK_MS は表示の更新間隔。秒を出すだけなので 250ms で十分。
 *  1000ms にすると、秒の変わり目が最大1秒ずれて見える。 */
const TICK_MS = 250;

const loadDuration = (): number =>
  clampDuration(readJSON<number>(restKeys.duration) ?? DEFAULT_DURATION_SEC);

const loadVolume = (): number =>
  clampVolume(readJSON<number>(restKeys.volume) ?? DEFAULT_VOLUME);

const loadState = (duration: number): RestState => {
  const saved = readJSON<RestState>(restKeys.state);
  if (saved?.kind === 'running' && typeof saved.startedAt === 'number') return saved;
  if (saved?.kind === 'paused' && typeof saved.remainingMs === 'number') return saved;
  return { kind: 'idle', durationSec: duration };
};

// useRestTimer は休憩タイマーを画面につなぐ。
//
// 状態は localStorage に残す。ジムではセットの合間に画面を消すので、
// 残さないと毎回タイマーが消えて使い物にならない。
export function useRestTimer() {
  const [durationSec, setDurationSecState] = useState(loadDuration);
  const [state, setState] = useState<RestState>(() => loadState(loadDuration()));
  const [now, setNow] = useState(() => Date.now());
  const [volume, setVolumeState] = useState(loadVolume);

  // 鳴らしたことを覚えておく。覚えないと、0 になったあと毎フレーム鳴る。
  const alertedFor = useRef<number | null>(null);

  const save = useCallback((next: RestState) => {
    setState(next);
    writeJSON(restKeys.state, next);
  }, []);

  const start = useCallback(() => {
    primeSound();
    void askNotificationPermission();
    const next: RestState = { kind: 'running', startedAt: Date.now(), durationSec };
    alertedFor.current = null;
    setNow(Date.now());
    save(next);
  }, [durationSec, save]);

  const pause = useCallback(() => {
    setState((s) => {
      if (s.kind !== 'running') return s;
      const next: RestState = {
        kind: 'paused',
        remainingMs: remainingMs(s, Date.now()),
        durationSec: s.durationSec,
      };
      writeJSON(restKeys.state, next);
      return next;
    });
  }, []);

  const resume = useCallback(() => {
    primeSound();
    setState((s) => {
      if (s.kind !== 'paused') return s;
      // 残り時間から逆算した開始時刻にする。こうすると running の計算が
      // 1つで済み、「一時停止をまたいだ経過」を別に持たなくてよい。
      const next: RestState = {
        kind: 'running',
        startedAt: Date.now() - (s.durationSec * 1000 - s.remainingMs),
        durationSec: s.durationSec,
      };
      writeJSON(restKeys.state, next);
      return next;
    });
  }, []);

  const reset = useCallback(() => {
    alertedFor.current = null;
    save({ kind: 'idle', durationSec });
  }, [durationSec, save]);

  const setDurationSec = useCallback(
    (sec: number) => {
      const next = clampDuration(sec);
      setDurationSecState(next);
      writeJSON(restKeys.duration, next);
      // 動いていないときは表示にも即座に効かせる。動いている最中に
      // 変えたら、次に始めたときから効く（走っているものを伸び縮みさせない）。
      setState((s) => (s.kind === 'idle' ? { kind: 'idle', durationSec: next } : s));
    },
    [],
  );

  // 音量は次に鳴る合図から効く。保存して、開き直しても残す。
  const setVolume = useCallback((next: number) => {
    const v = clampVolume(next);
    setVolumeState(v);
    writeJSON(restKeys.volume, v);
  }, []);

  // 試し聴き。音量を決めるには、実際に鳴らして聞くしかない。ボタンを押した
  // 操作の中で呼ぶので、primeSound で AudioContext も作れる。タイマーは動かさない。
  const previewSound = useCallback(() => {
    primeSound();
    beep(volume);
  }, [volume]);

  // 動いているあいだだけ時計を進める。止まっているのに再描画し続けない。
  useEffect(() => {
    if (state.kind !== 'running') return;
    const id = setInterval(() => setNow(Date.now()), TICK_MS);
    return () => clearInterval(id);
  }, [state.kind]);

  // 復帰したら即座に見直す。バックグラウンドのあいだ setInterval は
  // 止まるので、戻った瞬間に古い残り時間が見えるのを防ぐ。
  useEffect(() => {
    const onVisible = () => setNow(Date.now());
    document.addEventListener('visibilitychange', onVisible);
    window.addEventListener('focus', onVisible);
    return () => {
      document.removeEventListener('visibilitychange', onVisible);
      window.removeEventListener('focus', onVisible);
    };
  }, []);

  const left = remainingMs(state, now);
  const finished = state.kind === 'running' && left === 0;

  useEffect(() => {
    if (!finished || state.kind !== 'running') return;
    if (alertedFor.current === state.startedAt) return;
    alertedFor.current = state.startedAt;
    beep(volume);
    void notifyRestOver();
  }, [finished, state, volume]);

  return {
    state,
    durationSec,
    volume,
    remainingMs: left,
    finished,
    start,
    pause,
    resume,
    reset,
    setDurationSec,
    setVolume,
    previewSound,
  };
}

export type RestTimer = ReturnType<typeof useRestTimer>;
