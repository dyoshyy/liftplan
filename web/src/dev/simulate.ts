// 開発用シミュレーションの判断。
//
// 画面（.tsx）に書くと vitest が拾えない。問い合わせの組み立てと、
// 達成率の色分けはここに置く。

export type DevExercise = {
  id: string;
  name: string;
  derived_from?: string;
};

export type DevPreset = {
  key: string;
  name: string;
  days: string[];
};

export type DevOptions = {
  exercises: DevExercise[];
  presets: DevPreset[];
};

export type DevSet = {
  exercise_id: string;
  name: string;
  weight_kg: number | null;
  sets: number;
  target_rir: number;
  pct_of_1rm: number | null;
};

export type DevDay = {
  date: string;
  split: string;
  total_sets: number;
  main: DevSet[];
  variation: DevSet[];
  accessories: DevSet[];
};

export type DevWeek = {
  index: number;
  regions: { region: string; target: number; done: number }[];
};

export type DevResult = {
  days: DevDay[];
  weeks: DevWeek[];
};

/** Form は画面が持つ設定。 */
export type Form = {
  declared: string[];
  focus: string;
  split: string;
  frequency: number;
  weeks: number;
};

export const defaultForm: Form = {
  declared: ['bench', 'squat', 'deadlift'],
  focus: 'bench',
  split: 'upper_lower',
  frequency: 4,
  weeks: 4,
};

/** buildQuery は設定を問い合わせ文字列にする。空の項目は送らない。 */
export function buildQuery(form: Form): string {
  const q = new URLSearchParams();
  q.set('declared', form.declared.join(','));
  if (form.focus) q.set('focus', form.focus);
  if (form.split) q.set('split', form.split);
  q.set('frequency', String(form.frequency));
  q.set('weeks', String(form.weeks));
  return q.toString();
}

/** toggle は宣言種目の選び直し。並びは選んだ順に保つ。 */
export function toggle(list: string[], id: string): string[] {
  return list.includes(id) ? list.filter((v) => v !== id) : [...list, id];
}

/** withFocusInDeclared は重点種目が宣言から外れたときに指定を落とす。
 *
 *  外れたまま送るとサーバーが 400 を返す。画面の側で辻褄を合わせる。 */
export function withFocusInDeclared(form: Form): Form {
  return form.declared.includes(form.focus) ? form : { ...form, focus: '' };
}

/** rate は達成率。目標が0なら0を返す（0除算を画面に出さない）。 */
export function rate(done: number, target: number): number {
  return target > 0 ? done / target : 0;
}

/** Tone は達成率の色分け。許容は 60〜145%（通し検証と同じ帯）。 */
export type Tone = 'low' | 'ok' | 'high';

export function tone(value: number): Tone {
  if (value < 0.6) return 'low';
  if (value > 1.45) return 'high';
  return 'ok';
}

/** outOfRange は許容から外れた区分だけを返す。まずここを見る。 */
export function outOfRange(week: DevWeek): { region: string; value: number }[] {
  return week.regions
    .map((r) => ({ region: r.region, value: rate(r.done, r.target) }))
    .filter((r) => tone(r.value) !== 'ok');
}

/** formatWeight は重量の表示。未確定は本人が決める枠なのでその旨を出す。 */
export function formatWeight(set: DevSet): string {
  return set.weight_kg === null ? '自分で決める' : `${set.weight_kg}kg`;
}

/** formatPct は推定1RMに対する比。軸の一巡はこれを見て追う。 */
export function formatPct(set: DevSet): string {
  return set.pct_of_1rm === null ? '' : set.pct_of_1rm.toFixed(2);
}

/** declaredCandidates は宣言に選べる種目。派生は親の下にぶら下げて出す。 */
export function declaredCandidates(exercises: DevExercise[]): DevExercise[] {
  const roots = exercises.filter((e) => !e.derived_from);
  const derived = exercises.filter((e) => e.derived_from);
  return [...roots, ...derived];
}
