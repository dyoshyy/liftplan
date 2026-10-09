-- Hammer Strength のプリセットの名前から「（プレート）」を外す。
--
-- 汎用の種目（レッグプレスなど）と見分けるのは頭の「HS 」で足りる。末尾の
-- 「（プレート）」は一覧で名前を長くするだけなので外す（本人の依頼）。
--
-- 種目は利用者ごとにコピーされている（0014）ので、シードを直すだけでは既存の
-- 利用者に届かない。ID が hs_pl_ で始まり、名前が「（プレート）」で終わる行だけを
-- 直す。本人が付け直した名前（「（プレート）」で終わらない）は触らない。消した行も
-- 直す。戻したときに古い名前が蘇らないようにするため（0017 と同じ）。
--
-- 外した名前が、同じ利用者の消していない別の種目と同名になる行は直さない。
-- 名前の部分一意索引（user_exercises_alive_name）に当たって、このマイグレーション
-- ごと失敗する。
UPDATE user_exercises u
   SET name = left(u.name, length(u.name) - length('（プレート）'))
 WHERE u.id LIKE 'hs\_pl\_%'
   AND u.name LIKE '%（プレート）'
   AND NOT EXISTS (
        SELECT 1 FROM user_exercises a
         WHERE a.user_id = u.user_id
           AND a.id <> u.id
           AND a.deleted_at IS NULL
           AND a.name = left(u.name, length(u.name) - length('（プレート）')));
