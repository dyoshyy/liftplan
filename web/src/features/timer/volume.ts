// 休憩の終わりの合図の音量。React も DOM も知らない。
//
// 利用者が決めるのは 0〜100 の目盛り。実際に鳴らすときのピークの大きさ
// （Web Audio のゲイン）への換算と、何を鳴らすかの計画はここで決め、
// alert.ts は計画どおりに鳴らすだけにする。鳴らす側は AudioContext が要り、
// 単体では検査できないため。

/** DEFAULT_VOLUME は既定の音量。
 *
 *  peakGain が 0.25 になる値にしてある。音量を設定にする前は、ピークを 0.25 に
 *  固定していた。既定がこれと違うと、設定に触っていない人の合図の聞こえ方が
 *  黙って変わる。 */
export const DEFAULT_VOLUME = 50;

export const MIN_VOLUME = 0;
export const MAX_VOLUME = 100;

/** clampVolume は音量を 0〜100 の整数に収める。数でなければ既定に戻す。
 *
 *  保存した値が壊れていたときに、無音にも爆音にもしないため。 */
export function clampVolume(volume: number): number {
  if (typeof volume !== 'number' || !Number.isFinite(volume)) return DEFAULT_VOLUME;
  return Math.min(MAX_VOLUME, Math.max(MIN_VOLUME, Math.round(volume)));
}

/** parseVolume は入力欄の文字を音量にする。
 *
 *  数として読めない入力（空・途中の「-」）は、いまの値のままにする。
 *  打ち直している最中に、0 や既定へ飛ばさないため。 */
export function parseVolume(text: string, current: number): number {
  const n = Number.parseFloat(text);
  return Number.isFinite(n) ? clampVolume(n) : current;
}

/** peakGain は音量（0〜100）を音のピークの大きさ（0〜1）にする。
 *
 *  2乗で曲げる。線形だと、目盛りの下半分でほとんど差が聞こえない。耳は対数で
 *  聞くので、目盛りの刻みが同じくらいの差に聞こえるようにする。1 を超えると
 *  音が割れるので、上限は 1。 */
export function peakGain(volume: number): number {
  const v = clampVolume(volume) / MAX_VOLUME;
  return v * v;
}

/** volumeSummary は畳んだ節に出す現在値。0 は「0%」だと壊れて見えるので言葉にする。 */
export const volumeSummary = (volume: number): string =>
  volume === 0 ? '音なし' : `${volume}%`;

/** Tone は鳴らす音1つ。at は AudioContext の時計での開始時刻（秒）。 */
export type Tone = { at: number; peak: number };

const BEEP_COUNT = 3;
const BEEP_INTERVAL_SEC = 0.28;

/** beepTones は合図で鳴らす音の計画。now は AudioContext の現在時刻。
 *
 *  無音（0）なら音を1つも返さない。0 のゲインで発振器を回しても無駄で、
 *  「何も鳴らさない」を、予約する音が無いことで表すほうが取り違えない。 */
export function beepTones(volume: number, now: number): Tone[] {
  const peak = peakGain(volume);
  if (peak === 0) return [];
  return Array.from({ length: BEEP_COUNT }, (_, i) => ({ at: now + i * BEEP_INTERVAL_SEC, peak }));
}
