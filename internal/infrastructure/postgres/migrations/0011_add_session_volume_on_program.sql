-- 1セッションの量をプログラムに持たせる。
--
-- 既定は4種目×3セット。既存の行は「軸1＋補助8の9種目×3セット」で
-- 動いていたので、**挙動が変わる。**1日が27セットから12セットに減り、
-- 週目標もそれに合わせて下がる。
--
-- 既存の値を引き継がないのは、引き継ぐ先が無いため。9種目は新しい範囲
-- （2〜6）の外で、上限を入れること自体がこの変更の目的。
--
-- NOT NULL にするのは、未設定が正当な状態ではないから。頻度と同じく
-- 集約の不変条件で、NULL を許すと読み出しのたびに既定値を埋める分岐が要る。
ALTER TABLE program
  ADD COLUMN exercises_per_session integer NOT NULL DEFAULT 4,
  ADD COLUMN sets_per_exercise     integer NOT NULL DEFAULT 3;
