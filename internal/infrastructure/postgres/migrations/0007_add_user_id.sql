-- 記録に所有者を持たせる。マルチユーザー化の土台（2026-09-21 の仕様）。
--
-- このファイルだけで外から見た動作は変わらない。Go 側は user_id を
-- 1文字も書かないまま、これまでと同じ SQL で通る。所有者を実際に
-- 引数で貫通させるのは次のPR。
--
-- 既存の行は全て、下の既定ユーザーの UUID に寄せる。この UUID は
-- あとから本人の OAuth アカウントに結び直す（仕様の「既存の記録は
-- 既定ユーザーに寄せ、あとから本人のアカウントに結ぶ」）。
--
--
-- なぜ DEFAULT を付けるか（次のPRで外す）
-- ----------------------------------------
-- 付けないと、既存の INSERT 文が user_id を書いていないので NOT NULL に
-- 違反して落ちる。つまりこのマイグレーションと Go の書き換えを同じPRに
-- 入れることになり、「スキーマの移行」と「口の変更」が1本に混ざる。
--
-- DEFAULT があるうちは、所有者を渡し忘れた INSERT が**黙って既定ユーザーの
-- 行になる**。これは移行中だけ許す穴なので、リポジトリに UserID を貫通
-- させるPR（PR 3）で ALTER COLUMN ... DROP DEFAULT する。そこから先は
-- 渡し忘れが NOT NULL 違反として即座に出る。
--
--
-- なぜ主キーに所有者を含めるか
-- ----------------------------
-- set_logs.id はクライアントが採番する。user_id を列に足すだけで主キーを
-- id のままにすると、他人と同じIDを引いたときに**他人のデータへ手が届く**。
--
--   - B が A のIDで違う内容を送る → ErrConflictingSetLog が返る。
--     つまり「そのIDの記録が存在する」ことが他人に分かる
--   - B が A のIDを消す → A の記録が消える
--
-- 所有者を主キーに入れれば、この経路は SQL の形として塞がる。
-- daily_conditions も同じ理由で (user_id, date)。
--
--
-- なぜ旧い一意制約を1つ残すか（次のPRで外す）
-- --------------------------------------------
-- Go の INSERT は ON CONFLICT (id) / ON CONFLICT (date) を使う。Postgres は
-- 指定された列そのものに一意索引が無いと、この文を実行時に拒否する
-- （"no unique or exclusion constraint matching the ON CONFLICT specification"）。
-- 主キーを複合にした瞬間、(id) 単独の索引は消える。
--
-- そのままだと Go を書き換えねばならず、このPRの「動作不変」が成り立たない。
-- なので旧い形の UNIQUE を1つだけ残し、ON CONFLICT の受け皿にする。
-- いまは単一ユーザーしか居ないので、これが余計に禁じるものは無い。
-- ON CONFLICT の列を (user_id, id) に直す PR 3 で、この UNIQUE を落とす。
--
--
-- なぜ索引を作り直さないか
-- ------------------------
-- 0002 で set_logs_performed_on_idx と set_logs_exercise_idx を落としている。
-- set_logs を読む SQL は FindAll の1本だけで WHERE 句が無く、絞り込みは
-- 全て Go 側のメモリ上で行うため（D-034）。user_id を先頭に付けた索引を
-- いま作っても、走査されないまま INSERT ごとの更新コストを払うだけになる。
-- 読みが WHERE user_id = ... で絞るようになる PR 3 で、実際に使う形で作る。

-- set_logs -------------------------------------------------------------
ALTER TABLE set_logs
    ADD COLUMN user_id uuid NOT NULL DEFAULT '8d5e743e-f1b0-4430-9998-89d313e89da8';

ALTER TABLE set_logs DROP CONSTRAINT set_logs_pkey;
ALTER TABLE set_logs ADD PRIMARY KEY (user_id, id);
-- ON CONFLICT (id) の受け皿。PR 3 で落とす。
ALTER TABLE set_logs ADD CONSTRAINT set_logs_single_user_id_key UNIQUE (id);

-- daily_conditions -----------------------------------------------------
ALTER TABLE daily_conditions
    ADD COLUMN user_id uuid NOT NULL DEFAULT '8d5e743e-f1b0-4430-9998-89d313e89da8';

ALTER TABLE daily_conditions DROP CONSTRAINT daily_conditions_pkey;
ALTER TABLE daily_conditions ADD PRIMARY KEY (user_id, date);
-- ON CONFLICT (date) の受け皿。PR 3 で落とす。
ALTER TABLE daily_conditions ADD CONSTRAINT daily_conditions_single_user_date_key UNIQUE (date);

-- program --------------------------------------------------------------
--
-- id boolean PRIMARY KEY DEFAULT true CHECK (id) は「2行目を作れなくする」
-- ための列で、マルチユーザーになれば役目が終わる。ただし列ごと落とすと
-- Go の SELECT ... WHERE id と INSERT (id, ...) VALUES (true, ...) が
-- 同時に壊れるので、落とすのは PR 3。
--
-- それまでの間、CHECK (id) と UNIQUE (id) の組み合わせが「プログラムは
-- 全体で1行」をそのまま保つ。つまり移行中の不変条件は今までと同じ。
ALTER TABLE program
    ADD COLUMN user_id uuid NOT NULL DEFAULT '8d5e743e-f1b0-4430-9998-89d313e89da8';

ALTER TABLE program DROP CONSTRAINT program_pkey;
ALTER TABLE program ADD PRIMARY KEY (user_id);
-- ON CONFLICT (id) の受け皿。PR 3 で id 列ごと落とす。
ALTER TABLE program ADD CONSTRAINT program_single_user_id_key UNIQUE (id);
