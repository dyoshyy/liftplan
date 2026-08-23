-- バリエーション7種目を、選択済みの種目に追加する。
--
-- これまで `usablePool` に「選択されていなくてもバリエーションは使う」という
-- 抜け道があり、7種目は選択に入っていなくても回っていた。抜け道を塞いだので、
-- 明示的に選択されていないと出てこなくなる。
--
-- 追加しないと、種目が黙って7つ消える。
--
-- 既に入っているものは重複させない。jsonb の配列に対して、まだ無いものだけを
-- 連結する。

UPDATE program
SET selected = (
    SELECT jsonb_agg(id ORDER BY id)
    FROM (
        SELECT jsonb_array_elements_text(selected) AS id
        UNION
        SELECT unnest(ARRAY[
            'larsen_press',
            'tempo_bench',
            'close_grip_bench',
            'pause_squat',
            'front_squat',
            'deficit_deadlift',
            'romanian_deadlift'
        ]) AS id
    ) AS merged
);
