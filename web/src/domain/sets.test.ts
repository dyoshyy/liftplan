import { describe, expect, it } from 'vitest';
import { formatLast, formatSets, parseSetInput, usesBodyweight } from './sets';

describe('formatSets', () => {
  // 1セット目の重量で代表させると、落とした重量も上げた重量も履歴から消える。
  it.each([
    {
      name: '同じ重量はまとめる',
      sets: [
        { weight_kg: 100, reps: 8 },
        { weight_kg: 100, reps: 8 },
        { weight_kg: 100, reps: 7 },
      ],
      want: '100kg × 8, 8, 7',
    },
    {
      name: '重量の変わり目で区切る',
      sets: [
        { weight_kg: 100, reps: 8 },
        { weight_kg: 90, reps: 8 },
      ],
      want: '100kg × 8　/　90kg × 8',
    },
    {
      name: '同じ重量に戻ったら別の区切りになる',
      sets: [
        { weight_kg: 100, reps: 8 },
        { weight_kg: 90, reps: 8 },
        { weight_kg: 100, reps: 5 },
      ],
      want: '100kg × 8　/　90kg × 8　/　100kg × 5',
    },
    { name: '記録が無ければ空', sets: [], want: '' },
  ])('$name', ({ sets, want }) => {
    expect(formatSets(sets)).toBe(want);
  });
});

describe('formatLast', () => {
  it('セットごとの重量が無ければ代表の重量で埋める', () => {
    expect(formatLast({ weight_kg: 80, reps: [8, 8], days_ago: 3 })).toBe('80kg × 8, 8');
  });

  it('セットごとの重量があればそちらを使う', () => {
    expect(formatLast({ weight_kg: 80, weights: [80, 70], reps: [8, 10], days_ago: 3 })).toBe(
      '80kg × 8　/　70kg × 10',
    );
  });
});

describe('parseSetInput', () => {
  it('数として読めれば値を返す', () => {
    expect(parseSetInput('102.5', '8', '2')).toEqual({
      ok: true,
      values: { weight: 102.5, reps: 8, rir: 2 },
    });
  });

  // RIR 0 は「限界まで」で、正しい記録。0 を弾くと限界まで追い込んだ日が
  // 記録できない。
  it('RIR は 0 を通す', () => {
    expect(parseSetInput('100', '5', '0')).toEqual({ ok: true, values: { weight: 100, reps: 5, rir: 0 } });
  });

  it.each([
    { weight: '', reps: '8', rir: '2' },
    { weight: '100', reps: '', rir: '2' },
    { weight: '100', reps: '8', rir: '' },
  ])('空欄があれば止める（$weight / $reps / $rir）', ({ weight, reps, rir }) => {
    expect(parseSetInput(weight, reps, rir)).toEqual({
      ok: false,
      warning: '重量・レップ・RIR を入れてください',
    });
  });

  // 0kg や 0レップが通ると、推定1RM が 0 に引きずられる。
  it.each([
    { weight: '0', reps: '8', rir: '2' },
    { weight: '100', reps: '0', rir: '2' },
    { weight: '100', reps: '8', rir: '-1' },
  ])('0 以下は止める（$weight / $reps / $rir）', ({ weight, reps, rir }) => {
    expect(parseSetInput(weight, reps, rir)).toEqual({
      ok: false,
      warning: '0 より大きい重量とレップを入れてください',
    });
  });
});

// 自重種目は、何も付けずにやるのが普通の記録。0kg を弾くと、チンニングを自重で
// やった日は記録できない（記録シートの「記録する」が通らない）。
// 推定1RMが 0 に引きずられる心配は、サーバーが体重を足して読み替えるので無い。
describe('parseSetInput（自重種目）', () => {
  const bodyweight = { bodyweight: true };

  it('重量 0 を通す（何も付けない記録）', () => {
    expect(parseSetInput('0', '8', '2', bodyweight)).toEqual({
      ok: true,
      values: { weight: 0, reps: 8, rir: 2 },
    });
  });

  it('加重も通す', () => {
    expect(parseSetInput('10', '8', '2', bodyweight)).toEqual({
      ok: true,
      values: { weight: 10, reps: 8, rir: 2 },
    });
  });

  // 重量の 0 を許すのは「何も付けない」まで。レップ 0 は何もしていない。
  // 負の重量は存在しない（補助つきの種目は別の種目として持つ）。
  it.each([
    { weight: '-2.5', reps: '8', rir: '2' },
    { weight: '0', reps: '0', rir: '2' },
    { weight: '0', reps: '8', rir: '-1' },
  ])('それでも止める（$weight / $reps / $rir）', ({ weight, reps, rir }) => {
    expect(parseSetInput(weight, reps, rir, bodyweight)).toEqual({
      ok: false,
      warning: '重量は 0 以上、レップは 0 より大きい値を入れてください',
    });
  });

  // 通常の種目は 0kg を通さない。ここまで緩めると、重量の入れ忘れが
  // 0kg の記録として通る。
  it('自重でない種目は、今までどおり 0kg を止める', () => {
    expect(parseSetInput('0', '8', '2', { bodyweight: false }).ok).toBe(false);
    expect(parseSetInput('0', '8', '2').ok).toBe(false);
  });
});

// サーバーの load_offsets は自重を使う種目だけを載せる。載っていることが
// 「自重を使う種目」の印で、係数そのものは画面に渡っていない。
describe('usesBodyweight', () => {
  it.each<{ name: string; offsets: Record<string, number>; id: string; want: boolean }>([
    { name: '足す量がある種目は自重を使う', offsets: { pull_up: 66.5 }, id: 'pull_up', want: true },
    { name: '載っていない種目は使わない', offsets: { pull_up: 66.5 }, id: 'bench', want: false },
    { name: '0 は使わない（足す量が無い）', offsets: { pull_up: 0 }, id: 'pull_up', want: false },
    { name: '空なら使わない', offsets: {}, id: 'pull_up', want: false },
  ])('$name', ({ offsets, id, want }) => {
    expect(usesBodyweight(offsets, id)).toBe(want);
  });
});
