import type { Exercise } from '../../api/types';
import { regionLabel } from '../../domain/regions';

// 種目の編集（足す・直す）の判断。副作用は持たない。
//
// 入力の検査はサーバーも同じことをする（exercise.NewExercise）。ここにも
// 置くのは、押す前に何が足りないかを読めるようにするため。判断の最後の
// 砦はサーバーで、ここは案内でしかない。

// サーバーと同じ上限。変えるときは exercise.maxStimulusRegions・
// maxNameRunes・training.NewIncrement と揃える。寄与の下限 0.1 は
// docs/specs/2026-09-26-custom-exercises-design.md「各区分 0.1〜1.0」の
// 決めごとで、サーバーの exercise.NewExercise が課す下限とそのまま揃えて
// ある（training.NewContribution 自体の下限はもっと低い＝量子化後に0に
// 潰れない最小の正の値だが、それとは別にサーバーが 0.1 を課す）。画面が
// ゆるく通してもサーバーの 400 で弾かれるだけなので、ここも同じ値にする。
const MAX_NAME_RUNES = 40;
const MAX_REGIONS = 8;
const MIN_CONTRIBUTION = 0.1;
const MAX_CONTRIBUTION = 1.0;
const MAX_INCREMENT_KG = 50;

/** DEFAULT_INCREMENT_KG は刻みの初期値。プレート式マシンの多くが 2.5kg 刻み。 */
export const DEFAULT_INCREMENT_KG = 2.5;

/** ExerciseDraft は「種目を足す・直す」の書きかけ。 */
export type ExerciseDraft = {
  name: string;
  /** stimulus は区分ごとの寄与。載っていない区分は効かない。 */
  stimulus: Record<string, number>;
  incrementKg: number;
};

export const emptyDraft = (): ExerciseDraft => ({ name: '', stimulus: {}, incrementKg: DEFAULT_INCREMENT_KG });

/** draftOf は「直す」の初期値。寄与はそのまま持ってくる。
 *
 *  stimulus を複製するのは、下書きへの書き込みが元の種目（一覧の表示に
 *  まだ使われている）を書き換えないようにするため。 */
export const draftOf = (e: Exercise): ExerciseDraft => ({
  name: e.name,
  stimulus: { ...e.stimulus },
  incrementKg: e.increment_kg,
});

/** cycleRegion は区分のチップを押したときの次の状態。
 *
 *  無し → 1.0 → 0.5 → 無し と回る。**1.0・0.5 以外の値（プリセットの 0.7 など）
 *  の区分を押したときは 1.0 にする**（このサイクルに無い値を、無関係な
 *  0.5 に飛ばさないため）。 */
export const cycleRegion = (draft: ExerciseDraft, region: string): ExerciseDraft => {
  const stimulus = { ...draft.stimulus };
  const now = stimulus[region];
  if (now === undefined) stimulus[region] = 1.0;
  else if (now === 1.0) stimulus[region] = 0.5;
  else if (now === 0.5) delete stimulus[region];
  else stimulus[region] = 1.0;
  return { ...draft, stimulus };
};

/** setContribution は寄与の数値欄。0.1〜1.0 に丸めず、そのまま入れる
 *  （検証は draftProblem が担う）。 */
export const setContribution = (draft: ExerciseDraft, region: string, value: number): ExerciseDraft => ({
  ...draft,
  stimulus: { ...draft.stimulus, [region]: value },
});

/** draftProblem は送れない理由を返す。送れるなら null。 */
export const draftProblem = (draft: ExerciseDraft): string | null => {
  const name = draft.name.trim();
  if (name === '') return '名前を入れてください';
  // サーバーは rune 数で数える。length は UTF-16 の数なので絵文字で食い違う。
  if ([...name].length > MAX_NAME_RUNES) return `名前は${MAX_NAME_RUNES}文字までです`;

  const entries = Object.entries(draft.stimulus);
  // 寄与1.0の区分が無い種目は、分割のどの日にも入らない（exercise.hasFullContribution）。
  if (!entries.some(([, v]) => v === MAX_CONTRIBUTION)) return '寄与1.0の区分を1つ以上選んでください';
  if (entries.length > MAX_REGIONS) return `区分は合わせて${MAX_REGIONS}つまでです`;
  if (entries.some(([, v]) => !(v >= MIN_CONTRIBUTION && v <= MAX_CONTRIBUTION)))
    return '寄与は0.1〜1.0にしてください';

  if (!(draft.incrementKg > 0) || draft.incrementKg > MAX_INCREMENT_KG)
    return `刻みは0より大きく${MAX_INCREMENT_KG}kg以下にしてください`;
  return null;
};

/** draftBody は POST/PUT /api/exercises の本文。サーバーの exerciseDTO と対。
 *
 *  区分を並べるのは、同じ下書きから毎回同じ本文ができるようにするため。 */
export const draftBody = (draft: ExerciseDraft): { name: string; stimulus: Record<string, number>; increment_kg: number } => {
  const stimulus = Object.fromEntries(
    Object.entries(draft.stimulus).sort(([a], [b]) => (a < b ? -1 : a > b ? 1 : 0)),
  );
  return { name: draft.name.trim(), stimulus, increment_kg: draft.incrementKg };
};

/** aliveExercises は消していない種目だけを返す。
 *
 *  サーバーは消した種目も返す（履歴に名前を出すため）。一覧に出すと、
 *  選んだ瞬間に 400 で断られる。 */
export const aliveExercises = (exercises: readonly Exercise[]): Exercise[] => exercises.filter((e) => !e.deleted);

/** deleteBlockedReason は消せない理由を返す。消せるなら null。
 *
 *  サーバーも 409 で断る。押す前に読めたほうが、次に何をすればいいか分かる。 */
export const deleteBlockedReason = (declared: readonly string[], id: string): string | null =>
  declared.includes(id) ? '伸ばしたい種目から外すと削除できます' : null;

/** DeleteStep は「消す」ボタンを押したときの次の一手。
 *
 *  act: 'arm' は確認待ちに入るだけで、まだ消さない。'confirm' は確認待ちの
 *  種目をもう一度押したので、実際に消してよい。 */
export type DeleteStep = { pendingId: string | null; act: 'arm' | 'confirm' };

/** nextDeleteStep は「消す」ボタンを押したときの次の確認状態を決める。
 *
 *  直しに隣接した1タップでの誤爆を防ぐため（消した種目は直せない・戻せない）、
 *  同じ id を続けて2回押したときだけ 'confirm' を返す。確認待ち中に別の id を
 *  押したら、そちらを確認待ちにする（前の確認は流れる。一覧全体で確認待ちは
 *  常に0か1個で、複数行を同時に確認待ちにしない）。 */
export const nextDeleteStep = (pendingId: string | null, id: string): DeleteStep =>
  pendingId === id ? { pendingId: null, act: 'confirm' } : { pendingId: id, act: 'arm' };

/** stimulusSummary は一覧の1行に出す要約（例：`大腿四頭筋 1.0・臀筋 0.7・…`）。
 *
 *  寄与の大きい順に並べる。**同点は区分名（コード）の昇順**で決める。
 *  representativeRegion（domain/parts.ts）の同点処理と同じ理由：どう解いても
 *  表示上の違いしかないが、開き直すたびに順序が変わらないことは要る。 */
export const stimulusSummary = (stimulus: Record<string, number>): string =>
  Object.entries(stimulus)
    .sort(([a, va], [b, vb]) => vb - va || (a < b ? -1 : a > b ? 1 : 0))
    .map(([region, v]) => `${regionLabel(region)} ${v.toFixed(1)}`)
    .join('・');
