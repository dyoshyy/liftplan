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
  role?: string;
};

export type DeloadProposal = {
  reason: string;
  intensity_drop_pct: number;
  stalled_exercises: string[];
};

export type Session = {
  date: string;
  main: PlannedSet[];
  accessories: PlannedSet[];
  deload_proposal: DeloadProposal | null;
};

export type Exercise = {
  id: string;
  name: string;
  kind: string;
  increment_kg: number;
};

export type ExercisesResponse = { exercises: Exercise[] };

export type RecordedSet = {
  id: string;
  weight_kg: number;
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
};

export type TrendPoint = { date: string; kg: number };

export type Trend = {
  exercise_id: string;
  name: string;
  points: TrendPoint[];
  current_kg: number;
  change_kg: number;
};

export type Volume = {
  region: string;
  target_sets: number;
  done_sets: number;
};

export type StatsResponse = {
  from: string;
  to: string;
  trends: Trend[];
  weekly_volume: Volume[];
};

export type Program = {
  per_week: number;
  weekly_target: Record<string, number>;
  selected_exercises: string[];
  declared_exercises: string[];
};

export type SetLogInput = {
  id: string;
  date: string;
  exercise_id: string;
  weight_kg: number;
  reps: number;
  rir: number;
};

export type ConditionInput = {
  date: string;
  body_weight_kg?: number;
  sleep_hours?: number;
};
