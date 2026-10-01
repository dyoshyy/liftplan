import { describe, expect, it } from 'vitest';
import {
  beepTones,
  clampVolume,
  DEFAULT_VOLUME,
  parseVolume,
  peakGain,
  volumeSummary,
} from './volume';

describe('peakGain', () => {
  // 音量を足す前は、ピークを 0.25 に固定していた。既定の音量がこれと違うと、
  // 設定に触っていない人の合図の聞こえ方が黙って変わる。
  it('既定の音量は、これまでの音（ピーク 0.25）と同じ', () => {
    expect(peakGain(DEFAULT_VOLUME)).toBe(0.25);
  });

  it('0 は無音、100 は最大', () => {
    expect(peakGain(0)).toBe(0);
    expect(peakGain(100)).toBe(1);
  });

  // 音量の目盛りを線形にすると、下半分でほとんど差が聞こえない。耳は対数で
  // 聞くので、2乗で曲げて、目盛りの刻みが同じくらいの差に聞こえるようにする。
  it('大きくなるほど増え、目盛りの中ほどで線形より小さい', () => {
    expect(peakGain(30)).toBeLessThan(peakGain(60));
    expect(peakGain(60)).toBeLessThan(peakGain(90));
    expect(peakGain(50)).toBeLessThan(0.5);
  });

  // 範囲の外の値で1を超えると、音が割れる。
  it('範囲の外でも 0〜1 に収まる', () => {
    expect(peakGain(250)).toBe(1);
    expect(peakGain(-30)).toBe(0);
  });
});

describe('clampVolume', () => {
  it.each([
    { v: 0, want: 0 },
    { v: 100, want: 100 },
    { v: 37.6, want: 38 },
    { v: 150, want: 100 },
    { v: -5, want: 0 },
  ])('$v → $want', ({ v, want }) => {
    expect(clampVolume(v)).toBe(want);
  });

  // 壊れた値が保存されていても、そこで無音にしたり爆音にしたりしない。
  // 既定に戻す。
  it('数でないものは既定に戻す', () => {
    expect(clampVolume(Number.NaN)).toBe(DEFAULT_VOLUME);
    expect(clampVolume(Number.POSITIVE_INFINITY)).toBe(DEFAULT_VOLUME);
    expect(clampVolume('大きく' as unknown as number)).toBe(DEFAULT_VOLUME);
  });
});

describe('parseVolume', () => {
  it('入力を範囲に収めて返す', () => {
    expect(parseVolume('70', 50)).toBe(70);
    expect(parseVolume('250', 50)).toBe(100);
  });

  // 入力欄を空にした瞬間に無音（0）や既定に飛ぶと、打ち直している最中に
  // 設定が書き換わる。空や途中の入力は、いまの値のままにする。
  it('数として読めない入力は、いまの値のまま', () => {
    expect(parseVolume('', 70)).toBe(70);
    expect(parseVolume('-', 70)).toBe(70);
  });
});

describe('volumeSummary', () => {
  // 畳んでいるときに「0%」と出ると、壊れているように見える。
  it('0 は「音なし」、それ以外は割合', () => {
    expect(volumeSummary(0)).toBe('音なし');
    expect(volumeSummary(50)).toBe('50%');
  });
});

describe('beepTones', () => {
  // 3回鳴らす間隔と数も、これまでと同じであること。
  it('既定では、これまでと同じ3音を 0.28 秒おきに鳴らす', () => {
    const tones = beepTones(DEFAULT_VOLUME, 10);
    expect(tones).toHaveLength(3);
    expect(tones.map((t) => t.peak)).toEqual([0.25, 0.25, 0.25]);
    expect(tones[0]?.at).toBeCloseTo(10);
    expect(tones[1]?.at).toBeCloseTo(10.28);
    expect(tones[2]?.at).toBeCloseTo(10.56);
  });

  // 0 のとき 0 のゲインで発振器を回すと、無音の音を3つ予約するだけで無駄。
  // 何も鳴らさないことを、予約する音が無いことで表す。
  it('無音（0）なら、鳴らす音が無い', () => {
    expect(beepTones(0, 10)).toEqual([]);
  });

  it('音量が音のピークに効く', () => {
    expect(beepTones(100, 0).map((t) => t.peak)).toEqual([1, 1, 1]);
    expect(beepTones(20, 0)[0]?.peak).toBeCloseTo(0.04);
  });
});
