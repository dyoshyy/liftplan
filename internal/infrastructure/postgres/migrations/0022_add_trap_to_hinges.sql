-- RDL とデッドリフトに僧帽筋上部（と中部・前腕）の寄与を入れる。
--
-- どちらもバーの重さで肩が下に引かれ続け、それを僧帽筋上部が支える。
-- RDL には僧帽筋も前腕も入っておらず、デッドリフトは中部だけだった。
-- 支える働き方で、ローのように肩甲骨を動かす種目ほどは効かないので、
-- 上部を主に置き、中部は小さくする。
--
-- 種目は利用者ごとにコピーされている（0014）ので、シードを直すだけでは
-- 既存の利用者に届かない。シードの旧値のままの行だけを書き換える。本人が
-- 効き方を直した行（値が旧値と違う）は触らない。消した行も書き換える。
-- 戻したときに古い値が蘇らないようにするため（0017 と同じ）。
UPDATE user_exercises
   SET stimulus = '{"HAMSTRING": 1.0, "GLUTE": 0.7, "ERECTOR": 0.7, "TRAP_UPPER": 0.3, "TRAP_MID": 0.1, "FOREARM": 0.3}'::jsonb
 WHERE id = 'romanian_deadlift'
   AND stimulus = '{"HAMSTRING": 1.0, "GLUTE": 0.7, "ERECTOR": 0.7}'::jsonb;

UPDATE user_exercises
   SET stimulus = '{"HAMSTRING": 1.0, "GLUTE": 0.8, "ERECTOR": 1.0, "TRAP_UPPER": 0.4, "TRAP_MID": 0.2, "FOREARM": 0.4, "QUAD": 0.3, "LAT": 0.3}'::jsonb
 WHERE id = 'deadlift'
   AND stimulus = '{"HAMSTRING": 1.0, "GLUTE": 0.8, "ERECTOR": 1.0, "TRAP_MID": 0.4, "FOREARM": 0.4, "QUAD": 0.3, "LAT": 0.3}'::jsonb;
