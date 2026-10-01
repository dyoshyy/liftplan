-- 消した種目を、非表示の種目として戻す。
--
-- 種目は消せなくなった。使わない種目は「使う種目」から外して非表示にする
-- （docs/specs/2026-09-26-custom-exercises-design.md）。消した種目を一覧から
-- 見えなくしたままにすると、後から使いたくなっても戻す口が無い。
--
-- 消したときに「使う種目」から外してあるので、戻しても計画には出ない。
--
-- 同じ名前の消していない種目がある行は戻さない。名前の部分一意索引
-- （user_exercises_alive_name）に当たって、このマイグレーションごと失敗する。
-- 消した同名の行が複数あるときは、いちばん最近消したものだけを戻す。
UPDATE user_exercises
   SET deleted_at = NULL
 WHERE (user_id, id) IN (
        SELECT DISTINCT ON (d.user_id, d.name) d.user_id, d.id
          FROM user_exercises d
         WHERE d.deleted_at IS NOT NULL
           AND NOT EXISTS (
                SELECT 1 FROM user_exercises a
                 WHERE a.user_id = d.user_id
                   AND a.name = d.name
                   AND a.deleted_at IS NULL)
         ORDER BY d.user_id, d.name, d.deleted_at DESC
       );
