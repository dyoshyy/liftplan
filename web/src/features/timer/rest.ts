// 休憩タイマーの状態と計算。React も DOM も知らない。
//
// 経過は「いま − 開始時刻」で計算し直す。数え続ける実装にすると、
// 画面をバックグラウンドに回したあいだ JS のタイマーが止まるので、
// 復帰したときに止まっていた分だけ長く残る。ジムでは画面を消すので必ず踏む。

export type RestState =
  | { kind: 'idle'; durationSec: number }
  | { kind: 'running'; startedAt: number; durationSec: number }
  | { kind: 'paused'; remainingMs: number; durationSec: number };

/** DEFAULT_DURATION_SEC は既定の休憩。本人が変えられる。 */
export const DEFAULT_DURATION_SEC = 180;

/** MIN_DURATION_SEC / MAX_DURATION_SEC は入力の範囲。
 *  0 秒だと開始した瞬間に鳴り、10 分を超えるものは休憩ではない。 */
export const MIN_DURATION_SEC = 15;
export const MAX_DURATION_SEC = 600;

export function remainingMs(state: RestState, now: number): number {
  switch (state.kind) {
    case 'idle':
      return state.durationSec * 1000;
    case 'paused':
      return state.remainingMs;
    case 'running':
      return Math.max(0, state.durationSec * 1000 - (now - state.startedAt));
  }
}

/** formatRemaining は m:ss にする。
 *
 *  秒は切り上げる。残り 0.4 秒で「0:00」と出ると、鳴る前に終わったように
 *  見えて、もう一度押される。 */
export function formatRemaining(ms: number): string {
  const total = Math.ceil(Math.max(0, ms) / 1000);
  const m = Math.floor(total / 60);
  const s = total % 60;
  return `${m}:${String(s).padStart(2, '0')}`;
}

export const isFinished = (state: RestState, now: number): boolean =>
  state.kind === 'running' && remainingMs(state, now) === 0;

export const clampDuration = (sec: number): number =>
  Math.min(MAX_DURATION_SEC, Math.max(MIN_DURATION_SEC, Math.round(sec)));
