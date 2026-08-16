-- 実績ログ。エンジンにとって唯一の真実で、生成後は変更しない。
--
-- weight_kg を numeric にするのは、double precision だと 87.5kg のような値が
-- 往復で揺れうるため。ドメインは 1e-6 で量子化しているので numeric(6,2) で足りる。
CREATE TABLE set_logs (
    id           text         PRIMARY KEY,
    performed_on date         NOT NULL,
    exercise_id  text         NOT NULL,
    weight_kg    numeric(6,2) NOT NULL,
    reps         integer      NOT NULL,
    rir          integer      NOT NULL
);

-- 週内カバレッジとデロード判定が日付で絞る。
CREATE INDEX set_logs_performed_on_idx ON set_logs (performed_on);
-- 推定1RMと対メイン係数が種目で絞る。
CREATE INDEX set_logs_exercise_idx ON set_logs (exercise_id, performed_on);

-- 日次コンディション。体重と睡眠はどちらも欠損しうる。
CREATE TABLE daily_conditions (
    date           date PRIMARY KEY,
    body_weight_kg numeric(5,2),
    sleep_hours    numeric(4,2)
);

-- プログラムはユーザーごとに1つ。単一ユーザー前提なので1行に固定する。
-- id を true 固定の boolean にするのは、2行目を作れなくするため。
--
-- 週目標と選択種目を jsonb にするのは、列に開くと筋区分の追加で
-- マイグレーションが要るため。読み出しは必ずコンストラクタを通すので、
-- 不正な値が入っていても境界で弾かれる。
CREATE TABLE program (
    id            boolean PRIMARY KEY DEFAULT true CHECK (id),
    per_week      integer NOT NULL,
    weekly_target jsonb   NOT NULL,
    selected      jsonb   NOT NULL
);
