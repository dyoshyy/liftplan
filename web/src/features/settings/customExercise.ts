import type { Exercise } from '../../api/types';

// 自分の種目の判断。副作用は持たない。
//
// 入力の検査はサーバーも同じことをする（exercise.NewCustomExercise）。
// ここにも置くのは、押す前に何が足りないかを読めるようにするため。
// 判断の最後の砦はサーバーで、ここは案内でしかない。

/** RegionRole は、その部位にどう効くか。寄与の数値は持たない。
 *
 *  主は1.0、少しは0.5にサーバーが固定する。本人に数値を選ばせないため。 */
export type RegionRole = 'primary' | 'secondary';

/** CustomExerciseDraft は「種目を足す」の書きかけ。 */
export type CustomExerciseDraft = {
  name: string;
  /** roles は部位ごとの効き方。載っていない部位は効かない。 */
  roles: Partial<Record<string, RegionRole>>;
  incrementKg: number;
};

// サーバーと同じ上限。変えるときは exercise.maxCustomNameRunes・
// maxStimulusRegions・training.maxIncrementKg と揃える。
const MAX_NAME_RUNES = 40;
const MAX_REGIONS = 8;
const MAX_INCREMENT_KG = 50;

/** DEFAULT_INCREMENT_KG は刻みの初期値。プレート式マシンの多くが 2.5kg 刻み。 */
export const DEFAULT_INCREMENT_KG = 2.5;

export const emptyDraft = (): CustomExerciseDraft => ({ name: '', roles: {}, incrementKg: DEFAULT_INCREMENT_KG });

/** cycleRole は部位のチップを押したときの次の状態。無し → 主 → 少し → 無し。
 *
 *  主と少しを別の一覧にしないのは、同じ部位を両方に入れる操作をそもそも
 *  作らないため。 */
export const cycleRole = (draft: CustomExerciseDraft, region: string): CustomExerciseDraft => {
  const roles = { ...draft.roles };
  const now = roles[region];
  if (now === undefined) roles[region] = 'primary';
  else if (now === 'primary') roles[region] = 'secondary';
  else delete roles[region];
  return { ...draft, roles };
};

const regionsWith = (draft: CustomExerciseDraft, role: RegionRole): string[] =>
  Object.entries(draft.roles)
    .filter(([, r]) => r === role)
    .map(([region]) => region)
    .sort();

/** draftProblem は送れない理由を返す。送れるなら null。 */
export const draftProblem = (draft: CustomExerciseDraft): string | null => {
  const name = draft.name.trim();
  if (name === '') return '名前を入れてください';
  // サーバーは rune 数で数える。length は UTF-16 の数なので絵文字で食い違う。
  if ([...name].length > MAX_NAME_RUNES) return `名前は${MAX_NAME_RUNES}文字までです`;
  // 主に効く部位が無い種目は、分割のどの日にも入らない。
  if (regionsWith(draft, 'primary').length === 0) return '主に効く部位を1つ以上選んでください';
  if (Object.keys(draft.roles).length > MAX_REGIONS) return `部位は合わせて${MAX_REGIONS}つまでです`;
  if (!(draft.incrementKg > 0) || draft.incrementKg > MAX_INCREMENT_KG)
    return `刻みは0より大きく${MAX_INCREMENT_KG}kg以下にしてください`;
  return null;
};

/** draftBody は POST /api/exercises の本文。サーバーの addCustomExerciseDTO と対。 */
export const draftBody = (draft: CustomExerciseDraft) => ({
  name: draft.name.trim(),
  primary: regionsWith(draft, 'primary'),
  secondary: regionsWith(draft, 'secondary'),
  increment_kg: draft.incrementKg,
});

/** aliveExercises は消していない種目だけを返す。
 *
 *  サーバーは消した種目も返す（履歴に名前を出すため）。設定の一覧に出すと、
 *  選んだ瞬間に 400 で断られる。 */
export const aliveExercises = (exercises: readonly Exercise[]): Exercise[] =>
  exercises.filter((e) => !e.deleted);

/** deleteBlockedReason は消せない理由を返す。消せるなら null。
 *
 *  サーバーも 409 で断る。押す前に読めたほうが、次に何をすればいいか分かる。 */
export const deleteBlockedReason = (declared: readonly string[], id: string): string | null =>
  declared.includes(id) ? '伸ばしたい種目から外すと消せます' : null;
