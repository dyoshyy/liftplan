-- 利用者が足した種目。
--
-- 共通の種目（シード）はバイナリ同梱のまま DB には置かない。ここに入るのは
-- 利用者ごとの種目だけで、読み出すときにシードと合わせる
-- （docs/specs/2026-09-26-custom-exercises-design.md）。
--
-- 寄与の数値ではなく「主に効く」「少し効く」の区分を持つ。1.0と0.5の対応は
-- ドメイン（exercise.NewCustomExercise）が持ち、ここに漏らさない。
--
-- user_id に外部キーを張らないのは set_logs などと同じ（利用者の表が無く、
-- accounts の user_id は一意でない。0008）。
--
-- 消すのは論理削除。記録（set_logs.exercise_id）が種目を ID で指していて、
-- 行を消すと履歴から名前が消える。
CREATE TABLE custom_exercises (
    user_id           uuid        NOT NULL,
    id                text        NOT NULL,
    name              text        NOT NULL,
    primary_regions   jsonb       NOT NULL,
    secondary_regions jsonb       NOT NULL,
    increment_kg      numeric     NOT NULL,
    created_at        timestamptz NOT NULL DEFAULT now(),
    deleted_at        timestamptz,
    PRIMARY KEY (user_id, id)
);

-- 消していない種目の中で名前を一意にする。アプリでも弾くが、同時に2つ
-- 足したときの最後の砦。消した種目と同じ名前で足し直すのは許す。
CREATE UNIQUE INDEX custom_exercises_alive_name
    ON custom_exercises (user_id, name) WHERE deleted_at IS NULL;
