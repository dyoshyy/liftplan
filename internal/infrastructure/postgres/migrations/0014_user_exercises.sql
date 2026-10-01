-- 種目の利用者ごとの一覧。
--
-- 0013 の custom_exercises（旧版：共通の種目は同梱、本人が足した種目だけを
-- 主／少しの区分で持つ）を置き換える。0013 は本番に当たっているので書き換えず、
-- この表を足す。旧版で足した種目は、その人の初回の読み出しでプリセットと一緒に
-- 取り込む（postgres.ExerciseRepository.ensureSeeded）。custom_exercises は
-- 全員が取り込み終えたことを確かめてから別のマイグレーションで消す。
--
-- 種目は共通/個人の2つに分かれていない。プリセット（シード）は、その人の
-- 行が1件も無いときに一度だけこの表へコピーする。消した行も件数に数えるので、
-- 全部消してもプリセットが入り直らない。以後はこの表の行がその人の種目一覧
-- そのもの（docs/specs/2026-09-26-custom-exercises-design.md「いつコピーするか」）。
--
-- stimulus は筋区分ごとの寄与度を持つ（例 {"QUAD": 1.0, "GLUTE": 0.7}）。
-- 「主に効く」「少し効く」の2値ではなく、区分と数値の組をそのまま持つ。
-- 検証（0.1〜1.0・8区分まで・寄与1.0が1つ以上）は読み出しのたびに
-- exercise.NewExercise を通して行い、ここには漏らさない。
--
-- bodyweight_factor と derived_from はプリセット由来の値をコピーしたまま
-- 保つ。利用者が直せるのは名前・効き方・刻みだけ
-- （docs/specs/2026-09-26-custom-exercises-design.md「直せるもの | 名前、
-- 効き方（区分ごとの寄与）、刻み」）。
--
-- user_id に外部キーを張らないのは set_logs などと同じ（利用者の表が無く、
-- accounts の user_id は一意でない。0008）。
--
-- 消すのは論理削除。記録（set_logs.exercise_id）が種目を ID で指していて、
-- 行を消すと履歴から名前が消える。
CREATE TABLE user_exercises (
    user_id           uuid        NOT NULL,
    id                text        NOT NULL,
    name              text        NOT NULL,
    stimulus          jsonb       NOT NULL,
    increment_kg      numeric     NOT NULL,
    bodyweight_factor numeric     NOT NULL DEFAULT 0,
    derived_from      text,
    created_at        timestamptz NOT NULL DEFAULT now(),
    deleted_at        timestamptz,
    PRIMARY KEY (user_id, id)
);

-- 消していない種目の中で名前を一意にする。アプリでも弾くが、同時に2つ
-- 足したときの最後の砦。消した種目と同じ名前で足し直すのは許す。
CREATE UNIQUE INDEX user_exercises_alive_name
    ON user_exercises (user_id, name) WHERE deleted_at IS NULL;
