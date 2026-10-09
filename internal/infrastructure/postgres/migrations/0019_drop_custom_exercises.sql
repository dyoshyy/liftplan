-- 旧版の種目の表（0013 の custom_exercises）を消す。
--
-- 旧版で足した種目は、初回の読み出しで user_exercises に取り込んできた
-- （postgres.ExerciseRepository.ensureSeeded）。本番で全員の取り込みが済んだので
-- （2026-10-09 に確認。残っていた1行も取り込み済み）、取り込みの SQL ごと表を消す。
--
-- **表を消すと元に戻せない。**取り込み漏れの行（user_exercises に同じ
-- (user_id, id) が無い行）が1件でもあれば、消さずに失敗させる。サーバーは起動時に
-- マイグレーションを流すので、失敗すれば Deploy のヘルスチェックで止まり、
-- 古いリビジョンが動き続ける。
DO $$
DECLARE
    missing bigint;
BEGIN
    SELECT count(*) INTO missing
      FROM custom_exercises c
     WHERE NOT EXISTS (
            SELECT 1 FROM user_exercises u
             WHERE u.user_id = c.user_id AND u.id = c.id);
    IF missing > 0 THEN
        RAISE EXCEPTION 'custom_exercises に user_exercises へ取り込んでいない行が % 件ある。消さずに止める', missing;
    END IF;
END $$;

DROP TABLE custom_exercises;
