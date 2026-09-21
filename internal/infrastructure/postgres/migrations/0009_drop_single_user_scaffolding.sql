-- 単一ユーザー時代の足場を落とす。
--
-- 0007 は「Go を1行も変えずにスキーマだけ動かす」ために、3つの足場を
-- 残していた。このマイグレーションは、それを使っていた SQL を
-- 所有者込みに書き換えるのと同じPRで、足場のほうを落とす。
-- 0008 ではなく 0009 なのは、0008（accounts / sessions）が別PRで進んでいるため。
--
--   1. user_id の DEFAULT
--   2. 旧い形の UNIQUE（ON CONFLICT の受け皿）
--   3. program.id（1行固定のための boolean）
--
-- 順序に意味は無い。どれも同じトランザクションで流れる。


-- 1. DEFAULT を落とす --------------------------------------------------
--
-- DEFAULT があるうちは、所有者を渡し忘れた INSERT が**黙って既定ユーザーの
-- 行になる**。移行中だけ許していた穴で、Go が user_id を必ず書くように
-- なったいま、残す理由は無い。
--
-- 落とせば、渡し忘れは NOT NULL 違反として即座に出る。「静かに他人の
-- 行になる」より「その場で落ちる」ほうがよい。
ALTER TABLE set_logs         ALTER COLUMN user_id DROP DEFAULT;
ALTER TABLE daily_conditions ALTER COLUMN user_id DROP DEFAULT;
ALTER TABLE program          ALTER COLUMN user_id DROP DEFAULT;


-- 2. 旧い形の UNIQUE を落とす ------------------------------------------
--
-- ON CONFLICT の列が (user_id, id) / (user_id, date) / (user_id) に
-- 変わったので、受け皿は主キーの索引で足りる。
--
-- 残すと害がある。UNIQUE (id) は**利用者をまたいでIDの重複を禁じる**ので、
-- B が A と同じIDのログを送ると主キー違反ではなく一意制約違反で落ちる。
-- 「他人が使っているIDは使えない」は、IDの存在が他人に漏れる経路そのもの。
ALTER TABLE set_logs         DROP CONSTRAINT set_logs_single_user_id_key;
ALTER TABLE daily_conditions DROP CONSTRAINT daily_conditions_single_user_date_key;
ALTER TABLE program          DROP CONSTRAINT program_single_user_id_key;


-- 3. program.id を落とす -----------------------------------------------
--
-- 「2行目を作れなくする」ための列だった。主キーが user_id になったいま、
-- 「1人につき1行」はそちらが保つ。CHECK (id) も列と一緒に落ちる。
ALTER TABLE program DROP COLUMN id;


-- 索引を足さない理由 ---------------------------------------------------
--
-- 読みは全て WHERE user_id = $1 で絞るようになったが、user_id 単独の
-- 索引は要らない。主キーが (user_id, id) / (user_id, date) / (user_id) で、
-- どれも先頭列が user_id なので、その索引がそのまま使われる。
--
-- 足すと、走査されないまま INSERT ごとの更新コストだけを払う。
