-- 伸ばしたい種目ごとに、軸で狙うレップ数（重い番・軽い番）を持たせる。
--
-- 形は {"pull_up": {"heavy": 8, "light": 12}}。設定した宣言だけを持つ。
-- 既存の行は NULL＝どの宣言も既定（重い番3・軽い番6）で、挙動は変わらない。
-- 埋め戻しは要らない。
--
-- jsonb にするのは selected / declared / split_cycle と同じ理由で、要素数が
-- 可変なため。範囲（1〜15）は読み出しのたびに program.NewRepTargets が見る。
ALTER TABLE program ADD COLUMN declared_reps jsonb;
