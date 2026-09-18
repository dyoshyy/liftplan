import { describe, expect, it } from 'vitest';
import { remainingMs, type RestState } from './rest';

const running = (startedAt: number, durationSec: number): RestState => ({
  kind: 'running',
  startedAt,
  durationSec,
});

describe('remainingMs', () => {
  // 画面を閉じている間 JS のタイマーは止まる。経過は「いま − 開始時刻」で
  // 計算し直す。数え続ける実装だと、復帰したときに止まっていた分だけ長く出る。
  it('開始時刻からの実時間で計算する', () => {
    const t0 = 1_000_000;
    expect(remainingMs(running(t0, 180), t0)).toBe(180_000);
    expect(remainingMs(running(t0, 180), t0 + 60_000)).toBe(120_000);
  });

  it('過ぎていたら 0 で止める（負にしない）', () => {
    const t0 = 1_000_000;
    expect(remainingMs(running(t0, 180), t0 + 200_000)).toBe(0);
  });

  // 一時停止は「止めた時点の残り」を持つ。開始時刻を持ったままだと、
  // 止めているあいだも減り続ける。
  it('止めているあいだは減らない', () => {
    const paused: RestState = { kind: 'paused', remainingMs: 90_000, durationSec: 180 };
    expect(remainingMs(paused, 1_000_000)).toBe(90_000);
    expect(remainingMs(paused, 9_000_000)).toBe(90_000);
  });

  it('動いていなければ設定した長さを出す', () => {
    const idle: RestState = { kind: 'idle', durationSec: 180 };
    expect(remainingMs(idle, 1_000_000)).toBe(180_000);
  });
});

describe('formatRemaining', () => {
  it.each([
    { ms: 180_000, want: '3:00' },
    { ms: 125_000, want: '2:05' },
    { ms: 61_000, want: '1:01' },
    { ms: 9_000, want: '0:09' },
    { ms: 0, want: '0:00' },
    // 秒は切り上げる。残り 0.4 秒で「0:00」と出ると、鳴る前に終わったように見える。
    { ms: 400, want: '0:01' },
  ])('$ms ミリ秒 → $want', async ({ ms, want }) => {
    const { formatRemaining } = await import('./rest');
    expect(formatRemaining(ms)).toBe(want);
  });
});
