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
  intent?: string;
};

export type Session = {
  date: string;
  main: PlannedSet[];
  accessories: PlannedSet[];
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

export type ConditionInput = {
  date: string;
  body_weight_kg?: number;
  sleep_hours?: number;
};
