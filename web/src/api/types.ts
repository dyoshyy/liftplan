// サーバーの契約。internal/presentation/httpapi/dto.go と対になる。
//
// コード生成は入れていない。エンドポイントが9つで、生成器を足すほうが重い。
// dto.go を変えたらここも変える。同じリポジトリに置いてあるのは、
// この追従を PR の中で見えるようにするため。

export type PlannedSet = {
  exercise_id: string;
  /** null は「自分で決める」。推定1RM が古いか、記録が足りない。 */
  weight_kg: number | null;
  sets: number;
  target_rir: number;
  /** 狙うレップ数。重量が null でも来る。 */
  target_reps: number;
};

/**
 * 3レーン。軸は必ず1件、バリエーションは高々1件、補助は0件以上。
 * 出ない日は null ではなく空配列で来る（dto.go）。
 */
export type Session = {
  date: string;
  main: PlannedSet[];
  variation: PlannedSet[];
  accessories: PlannedSet[];
};

/** 見込みの1回ぶん。dto.go の forecastSessionDTO と対。date は無い
 *  （画面では出さない決定なので、応答にも乗せない）。 */
export type ForecastSession = {
  index: number;
  /** split はその回の分割の日の名前。分割なしは null。 */
  split: string | null;
  main: PlannedSet[];
  variation: PlannedSet[];
  accessories: PlannedSet[];
};

export type ForecastResponse = { sessions: ForecastSession[] };

/** プログラム。dto.go の programDTO と対。 */
/** 分割1件。区分が空なら全区分（全身法の日）。 */
export type Split = {
  name: string;
  regions: string[];
};

/** 分割のプリセット。並びが周期そのもの。 */
export type SplitPreset = {
  key: string;
  name: string;
  splits: Split[];
  /** このプリセットを選ぶために必要な週の最小頻度。0 は下限なし。 */
  min_frequency_per_week: number;
};

export type SplitPresetsResponse = { presets: SplitPreset[] };

/** Login はログイン方法の1つ。email は取っていなければ null。 */
export type Login = { provider: string; email: string | null };

export type AccountResponse = { accounts: Login[] };

export type Program = {
  per_week: number;
  /** 1回に出る種目の数（2〜6）。利用者が時間に合わせて選ぶ。 */
  exercises_per_session: number;
  /** 1種目あたりのセット数（2〜6）。 */
  sets_per_exercise: number;
  selected_exercises: string[];
  declared_exercises: string[];
  /** null は指定なし。指定すると、その種目の派生がバリエーションレーンに出る。 */
  focus_exercise: string | null;
  /** 分割の周期。空なら分割なし。並びに意味がある。 */
  splits: Split[];
};

export type Exercise = {
  id: string;
  name: string;
  increment_kg: number;
  /** stimulus は各筋区分への刺激。画面が種目を部位ごとにまとめるのに使う。
   *  どれを代表に選ぶかは表示の判断なので、サーバーは分布のまま返す。 */
  stimulus: Record<string, number>;
  /** deleted は消した種目か。履歴の名前のために返ってくる。設定の一覧には出さない。 */
  deleted?: boolean;
};

export type ExercisesResponse = { exercises: Exercise[] };

export type RecordedSet = {
  id: string;
  /** weight_kg は記録した加重。自重なら 0。 */
  weight_kg: number;
  /**
   * effective_kg は、そのセットの日の体重を足した負荷（自重種目だけ。通常は weight_kg と同じ）。
   * 0 は体重が引けず推定できない。サーバーから来たセットだけが持つ（手元で記録した
   * 今日のセットには無い。いま足す量 `load_offsets` で読み替える）。
   */
  effective_kg?: number;
  reps: number;
  rir: number;
};

export type ExerciseLog = {
  exercise_id: string;
  name: string;
  sets: RecordedSet[];
};

export type Day = {
  date: string;
  exercises: ExerciseLog[];
  total_sets: number;
};

export type LastPerformance = {
  date: string;
  weight_kg: number;
  weights: number[];
  reps: number[];
  days_ago: number;
};

export type HistoryResponse = {
  from: string;
  to: string;
  days: Day[];
  last_performances: Record<string, LastPerformance>;
  /** 種目ごとの、記録した加重に足すと体重込みの負荷になる量。自重を使う種目だけ。 */
  load_offsets: Record<string, number>;
};

export type ConditionInput = {
  date: string;
  body_weight_kg?: number;
  sleep_hours?: number;
};

/** 推定1RM の1点。dto.go の pointDTO と対。 */
export type TrendPoint = { date: string; kg: number };

/** 種目ごとの推定1RM の推移。dto.go の trendDTO と対。 */
export type Trend = {
  exercise_id: string;
  name: string;
  points: TrendPoint[];
  current_kg: number;
  change_kg: number;
};

/** 筋区分ごとの週の充足。dto.go の volumeDTO と対。 */
export type Volume = {
  region: string;
  target_sets: number;
  done_sets: number;
};

/** dto.go の statsResponse と対。 */
export type StatsResponse = {
  from: string;
  to: string;
  trends: Trend[];
  weekly_volume: Volume[];
};
