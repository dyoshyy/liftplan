// シミュレーション画面の判断。
//
// 画面（.tsx）に書くと vitest が拾えない。問い合わせの組み立てと、
// 達成率の色分けと、失敗の読み替えはここに置く。

import { Unauthorized } from '../api/client';

export type DevExercise = {
  id: string;
  name: string;
  derived_from?: string;
  /** 模擬ユーザーの初日の実力の既定値（加重の1RM）。 */
  default_1rm_kg: number;
  /** 自重が負荷に乗る種目か。1RM は加重の分だけで表す。 */
  bodyweight: boolean;
};

export type DevPreset = {
  key: string;
  name: string;
  days: string[];
};

export type DevAthlete = {
  growth_pct_per_week: number;
  first_session_pct: number;
  body_weight_kg: number;
  /** 応答の settings では全種目ぶん入る。options では無い。 */
  one_rep_max_kg?: Record<string, number>;
};

export type DevSchedule = {
  exercises_per_session: number;
  sets_per_exercise: number;
  /** 頻度（"1"〜"7"）ごとの既定の曜日（開始日からの日数）。 */
  weekdays_by_frequency: Record<string, number[]>;
  /** 開始日の既定（YYYY-MM-DD）。 */
  start: string;
};

export type DevOptions = {
  exercises: DevExercise[];
  presets: DevPreset[];
  athlete_defaults: DevAthlete;
  schedule_defaults: DevSchedule;
};

export type DevSet = {
  exercise_id: string;
  name: string;
  weight_kg: number | null;
  sets: number;
  target_rir: number;
  pct_of_1rm: number | null;
  /** その日の模擬ユーザーの実力（加重の1RM）。 */
  athlete_1rm_kg: number;
  /** 模擬ユーザーが記録した値（全セット同じ）。処方が無い日も埋まる。 */
  performed: { weight_kg: number; reps: number; rir: number };
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

/** DevSettings は結果を作った設定。既定値を解決したあとの値。 */
export type DevSettings = {
  declared: string[];
  focus: string;
  split: string;
  frequency: number;
  weeks: number;
  start: string;
  athlete: DevAthlete;
  /** 通った曜日（開始日からの日数）。 */
  weekdays: number[];
  exercises_per_session: number;
  sets_per_exercise: number;
  /** 足した自分の種目。ID は orm= の上書きで指すのに使う。 */
  custom?: DevCustom[];
};

/** DevCustom は自分の種目の1件。dev_simulation.go の devCustomDTO と対。 */
export type DevCustom = {
  id: string;
  name: string;
  /** 区分ごとの寄与度の生の値（例: {"TRAP_MID": 1, "LAT": 0.5}）。 */
  stimulus: Record<string, number>;
  increment_kg: number;
};

export type DevResult = {
  settings: DevSettings;
  days: DevDay[];
  weeks: DevWeek[];
};

/** Form は画面が持つ設定。
 *
 *  模擬ユーザーの項目は null なら「サーバーの既定」。既定値はサーバーが
 *  持ち（/api/dev/options）、画面は二重に持たない。orm は既定値から
 *  変えた種目だけを持つ。 */
export type Form = {
  declared: string[];
  focus: string;
  split: string;
  frequency: number;
  weeks: number;
  growth: number | null;
  firstPct: number | null;
  bodyWeight: number | null;
  orm: Record<string, number>;
  /** 1回の種目数とセット数。null はサーバーの既定。 */
  exercises: number | null;
  sets: number | null;
  /** 通う曜日（開始日からの日数）。null は頻度ごとの既定。 */
  days: number[] | null;
  /** 開始日（YYYY-MM-DD）。null はサーバーの既定。 */
  start: string | null;
  /** 自分の種目。サーバーと同じ「名前|区分:寄与,区分:寄与|刻み」を ; で並べた
   *  1行のまま持つ。空なら足さない。 */
  custom: string;
};

export const defaultForm: Form = {
  declared: ['bench', 'squat', 'deadlift'],
  focus: 'bench',
  split: 'upper_lower',
  frequency: 4,
  weeks: 4,
  growth: null,
  firstPct: null,
  bodyWeight: null,
  orm: {},
  exercises: null,
  sets: null,
  days: null,
  start: null,
  custom: '',
};

/** buildQuery は設定を問い合わせ文字列にする。空の項目は送らない。
 *
 *  この文字列は画面の URL にもそのまま載せる（API と同じキー）。URL を
 *  開けば同じ設定で結果が出るので、Claude が URL 1本で状況を再現できる。 */
export function buildQuery(form: Form): string {
  const q = new URLSearchParams();
  q.set('declared', form.declared.join(','));
  // 空も送る。キーごと落とすと、URL を読み戻したときに既定に戻る。
  q.set('focus', form.focus);
  q.set('split', form.split);
  // 曜日を指定したら頻度はその数。食い違うとサーバーが 400 を返す。
  q.set('frequency', String(form.days?.length ?? form.frequency));
  q.set('weeks', String(form.weeks));
  if (form.days !== null) q.set('days', form.days.join(','));
  if (form.exercises !== null) q.set('exercises', String(form.exercises));
  if (form.sets !== null) q.set('sets', String(form.sets));
  if (form.start !== null) q.set('start', form.start);
  // 模擬ユーザーは触った項目だけ。null はサーバーの既定に任せる。
  if (form.growth !== null) q.set('growth', String(form.growth));
  if (form.firstPct !== null) q.set('first_pct', String(form.firstPct));
  if (form.bodyWeight !== null) q.set('body_weight', String(form.bodyWeight));
  const orm = Object.keys(form.orm)
    .sort()
    .map((id) => `${id}:${form.orm[id]}`);
  if (orm.length > 0) q.set('orm', orm.join(','));
  if (form.custom.trim() !== '') q.set('custom', form.custom.trim());
  return q.toString();
}

/** num は数字として読めれば返す。空や数字でないものは null。 */
function num(v: string | null): number | null {
  if (v === null || v.trim() === '') return null;
  const n = Number(v);
  return Number.isFinite(n) ? n : null;
}

/** parseForm は URL の問い合わせ文字列を設定に読み戻す。buildQuery の逆。
 *
 *  読めない値は既定に倒す。範囲はサーバーが見て、400 で理由を返す。
 *  split と focus は「空」が意味を持つ（分割なし・重点なし）ので、
 *  キーがあれば空でもそのまま使う。 */
export function parseForm(search: string, fallback: Form): Form {
  const q = new URLSearchParams(search);
  const orm: Record<string, number> = {};
  for (const pair of (q.get('orm') ?? '').split(',')) {
    const [id, kg] = pair.split(':');
    const n = num(kg ?? null);
    if (id && n !== null) orm[id] = n;
  }

  return {
    declared: q.has('declared')
      ? (q.get('declared') ?? '').split(',').filter((v) => v !== '')
      : fallback.declared,
    focus: q.has('focus') ? (q.get('focus') ?? '') : fallback.focus,
    split: q.has('split') ? (q.get('split') ?? '') : fallback.split,
    frequency: num(q.get('frequency')) ?? fallback.frequency,
    weeks: num(q.get('weeks')) ?? fallback.weeks,
    growth: q.has('growth') ? num(q.get('growth')) : fallback.growth,
    firstPct: q.has('first_pct') ? num(q.get('first_pct')) : fallback.firstPct,
    bodyWeight: q.has('body_weight') ? num(q.get('body_weight')) : fallback.bodyWeight,
    orm: q.has('orm') ? orm : fallback.orm,
    exercises: q.has('exercises') ? num(q.get('exercises')) : fallback.exercises,
    sets: q.has('sets') ? num(q.get('sets')) : fallback.sets,
    days: q.has('days') ? days(q.get('days') ?? '') : fallback.days,
    start: q.get('start') || fallback.start,
    custom: q.has('custom') ? (q.get('custom') ?? '') : fallback.custom,
  };
}

/** days は "1,3" を曜日の並びにする。1つでも読めなければ null（既定）。 */
function days(v: string): number[] | null {
  const out = v.split(',').map((d) => num(d));
  return out.length > 0 && out.every((d) => d !== null) ? (out as number[]) : null;
}

/** toggleDay は通う曜日を1つ切り替え、頻度をその数に揃える。
 *
 *  未指定（null）なら頻度ごとの既定の曜日を起点にする。空から始めると、
 *  1つ押しただけで既定の曜日が全部外れる。最後の1日は外さない（0日は
 *  頻度として成り立たない）。 */
export function toggleDay(form: Form, day: number, defaults: Record<string, number[]>): Form {
  const current = form.days ?? defaults[String(form.frequency)] ?? [];
  const next = current.includes(day)
    ? current.filter((d) => d !== day)
    : [...current, day].sort((a, b) => a - b);
  if (next.length === 0) return form;
  return { ...form, days: next, frequency: next.length };
}

/** setOneRepMax は種目の1RMを変える。既定値に戻したら上書きを消す。
 *
 *  残すと URL に既定値が並び、どれを変えたのかが読めなくなる。 */
export function setOneRepMax(
  orm: Record<string, number>,
  id: string,
  kg: number,
  fallback: number,
): Record<string, number> {
  const { [id]: _, ...rest } = orm;
  if (!Number.isFinite(kg) || kg === fallback) return rest;
  return { ...rest, [id]: kg };
}

/** curlCommand は同じ設定の結果を JSON で取る1行。画面を通さずに読むため。
 *
 *  トークンは環境変数のまま出す。画面に値を書き出さない。 */
export function curlCommand(base: string, form: Form): string {
  return `curl -s -H "Authorization: Bearer $LIFTPLAN_TOKEN" '${base}/api/dev/simulate?${buildQuery(form)}'`;
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
  if (set.weight_kg === null) return '自分で決める';
  return set.weight_kg === 0 ? '自重' : `${set.weight_kg}kg`;
}

/** formatPerformed は模擬ユーザーの記録。加重0は自重とだけ出す。 */
export function formatPerformed(set: DevSet): string {
  const p = set.performed;
  const kg = p.weight_kg === 0 ? '自重' : `${p.weight_kg}kg`;
  return `${kg}×${p.reps} RIR${p.rir}`;
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

/** describeFailure は失敗を画面に出す1行にする。
 *
 *  401 だけ扱いが違う。ほかの失敗は「もう一度押す」で直りうるが、これは
 *  直らない。トークンは捨てられた後（client.send）なので、この画面で
 *  ログインし直す手段は無い。行き先を言わないと、押し続けるしかなくなる。
 *
 *  それ以外はサーバーが書いた文をそのまま出す。入力が成り立たない理由は
 *  サーバーしか知らない（どの分割にどの区分があるか、頻度の上限）。
 *  握り潰すと画面に残るのは状態コードだけになる。 */
export function describeFailure(e: unknown): string {
  if (e instanceof Unauthorized) {
    return 'ログインが切れている。メイン画面（/）でログインし直してから開くこと';
  }
  return e instanceof Error ? e.message : String(e);
}
