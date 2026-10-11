import type { Program, SplitPreset } from '../../api/types';
import { isSplitSelectable, matchingPresetKey, splitBody } from '../settings/split';

// 初回の設定の判断。副作用は持たない。

/** Draft は初回の設定で選んでいる途中の値。「はじめる」まで送らない。 */
export type Draft = {
  perWeek: number;
  /** null は全身法。 */
  split: SplitPreset | null;
  exercises: number;
  sets: number;
  declared: string[];
};

/** STEPS は初回の設定の段階。この順に出す。 */
export const STEPS = ['frequency', 'split', 'volume', 'declared'] as const;
export type Step = (typeof STEPS)[number];

/** PutRequest は送る1件。本文の無いものは body を持たない。 */
export type PutRequest = { path: string; body?: unknown };

/** draftFrom はサーバーの現在値から下書きを作る。「次へ」を押し続けるだけで終われるように。 */
export function draftFrom(program: Program, presets: readonly SplitPreset[]): Draft {
  const key = matchingPresetKey(program, presets);
  return {
    perWeek: program.per_week,
    split: presets.find((p) => p.key === key) ?? null,
    exercises: program.exercises_per_session,
    sets: program.sets_per_exercise,
    declared: [...program.declared_exercises],
  };
}

/**
 * withFrequency は回数を変えた下書きを返す。
 *
 * その回数で選べなくなった分割は全身法に戻す。残すと、段階2に戻らない
 * かぎり気づけないまま「はじめる」でサーバーに断られる。
 */
export function withFrequency(draft: Draft, perWeek: number): Draft {
  const split = draft.split && isSplitSelectable(draft.split, perWeek) ? draft.split : null;
  return { ...draft, perWeek, split };
}

/**
 * planOnboardingRequests は「はじめる」で送る列を決める。最後は必ず完了の記録。
 *
 * 回数と分割はサーバーが両方向で検査する（今の分割に足りない回数も、今の
 * 回数で選べない分割も断る）。新しい分割が今の回数で選べるなら分割を先に、
 * そうでなければ回数を先に送る。後者のとき、新しい回数は新しい分割の下限
 * 以上で、それは今の回数より大きく、今の分割の下限（≤ 今の回数）も満たす。
 * どちらかの順は必ず通る。
 *
 * 変わっていない項目も送る。どれも冪等で、差分を取ると失敗後の再送で
 * 「どれが済んだか」を持つことになる。
 */
export function planOnboardingRequests(current: Program, draft: Draft): PutRequest[] {
  const frequency = { path: '/api/program/frequency', body: { per_week: draft.perWeek } };
  const split = { path: '/api/program/split', body: splitBody(draft.split) };
  const splitFirst = (draft.split?.min_frequency_per_week ?? 0) <= current.per_week;
  return [
    ...(splitFirst ? [split, frequency] : [frequency, split]),
    {
      path: '/api/program/volume',
      body: { exercises_per_session: draft.exercises, sets_per_exercise: draft.sets },
    },
    { path: '/api/program/declared', body: { declared_exercises: [...draft.declared].sort() } },
    { path: '/api/onboarding/completed' },
  ];
}
