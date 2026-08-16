-- 桁数をドメインの量子化に合わせる。
--
-- numeric(6,2) はドメインが受け付ける値を保持できない。ドメインは
-- 小数6桁まで量子化するので、87.125kg は正当な入力として通る。
-- 2桁に丸めると 87.13 で保存され、同じ内容を再送しても
-- 「内容が違う」と判定されて 409 になる。保存に成功したセットが
-- 二度と再送できなくなり、削除の口も無いので復旧できない。
ALTER TABLE set_logs        ALTER COLUMN weight_kg      TYPE numeric(10,6);
ALTER TABLE daily_conditions ALTER COLUMN body_weight_kg TYPE numeric(9,6);
ALTER TABLE daily_conditions ALTER COLUMN sleep_hours    TYPE numeric(8,6);

-- 使われていない索引を落とす。
--
-- set_logs を読む SQL は FindAll の1本だけで、WHERE 句が無い。
-- 日付や種目での絞り込みは全て Go 側のメモリ上で行う（D-034）。
-- 走査回数ゼロの索引は、INSERT ごとの更新コストを払うだけになる。
-- 範囲クエリへ移るときに、実際に使う形で作り直す。
DROP INDEX IF EXISTS set_logs_performed_on_idx;
DROP INDEX IF EXISTS set_logs_exercise_idx;
