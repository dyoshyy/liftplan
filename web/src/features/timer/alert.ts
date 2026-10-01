// 休憩の終わりを知らせる。音と通知。
//
// **画面を閉じている間の発火は保証できない。**PWA をバックグラウンドに
// 回すと JS は止まる。復帰すれば「もう過ぎている」ことは必ず分かるが、
// ポケットの中で鳴る保証は無い（Push を入れない限り）。ここは割り切る。

import { beepTones } from './volume';

let audio: AudioContext | null = null;

/** primeSound は利用者の操作の中で呼ぶ。
 *
 *  ブラウザは操作を伴わない音を止めるので、記録ボタンを押した流れの中で
 *  AudioContext を作っておく。0秒になってから作ると鳴らない。 */
export function primeSound(): void {
  try {
    audio ??= new AudioContext();
    if (audio.state === 'suspended') void audio.resume();
  } catch {
    // 音が出ないだけ。タイマー自体は動く。
  }
}

/** beep は短い音を鳴らす。何を何回鳴らすかは beepTones が決める（volume.ts）。
 *
 *  音源のファイルを持たないのは、オフラインでも鳴らすため。殻に載せると
 *  プリキャッシュが増えるうえ、鳴らない端末では無駄になる。 */
export function beep(volume: number): void {
  if (!audio) return;
  try {
    for (const { at, peak } of beepTones(volume, audio.currentTime)) {
      const osc = audio.createOscillator();
      const gain = audio.createGain();
      osc.type = 'sine';
      osc.frequency.value = 880;
      // 立ち上がりと減衰を付ける。矩形に切ると耳障りなクリックが入る。
      gain.gain.setValueAtTime(0, at);
      gain.gain.linearRampToValueAtTime(peak, at + 0.02);
      gain.gain.exponentialRampToValueAtTime(0.0001, at + 0.22);
      osc.connect(gain).connect(audio.destination);
      osc.start(at);
      osc.stop(at + 0.24);
    }
  } catch {
    // 鳴らないだけ。
  }
}

/** askNotificationPermission はタイマーを初めて開始したときに呼ぶ。
 *
 *  起動時に求めない。何もしていないうちに許可を聞かれると、何のための
 *  通知か分からないまま拒否される。 */
export async function askNotificationPermission(): Promise<void> {
  if (!('Notification' in window)) return;
  if (Notification.permission !== 'default') return;
  try {
    await Notification.requestPermission();
  } catch {
    // 求められないだけ。
  }
}

export async function notifyRestOver(): Promise<void> {
  if (!('Notification' in window) || Notification.permission !== 'granted') return;
  const body = '次のセットへ';
  try {
    // Service Worker 経由のほうが、画面が前に無いときに出やすい。
    const reg = await navigator.serviceWorker?.getRegistration();
    if (reg) {
      await reg.showNotification('休憩おわり', { body, tag: 'liftplan-rest', icon: '/icon.svg' });
      return;
    }
    new Notification('休憩おわり', { body, tag: 'liftplan-rest', icon: '/icon.svg' });
  } catch {
    // 通知が出ないだけ。画面には出ている。
  }
}
