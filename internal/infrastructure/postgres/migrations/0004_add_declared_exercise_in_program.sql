-- 「伸ばしたい種目」の列を足す（D-117）。
--
-- 軸を筋肉群の枠として持つのをやめ、Program が「この種目を伸ばしたい」
-- という宣言の集合を持つようにした。これまで種目マスタの Kind == MAIN が
-- 担っていた役割で、既存行にはその顔ぶれ（BIG3）をそのまま入れる。
--
-- 3段階に分けるのは、一発で NOT NULL にすると既存行のあるDBで失敗するため。
-- 列を足した直後、その値は NULL なので、NOT NULL 制約に違反する。

-- 1. まず NULL を許して列を足す。
ALTER TABLE program ADD COLUMN declared jsonb;

-- 2. 既存行を埋める。
--
-- 移行前に「メイン種目」として毎回出ていた3つ。ここを変えると、
-- 移行した瞬間にメニューの中身が変わる。
--
-- WHERE を付けるのは、二度流しても既存の値を上書きしないため。
-- 新規のDBではプログラムがまだ無いので0行になる。それが正しい。
-- 初期プログラムは cmd/api の defaultProgram が組む。
UPDATE program SET declared = '["bench", "squat", "deadlift"]'::jsonb
WHERE declared IS NULL;

-- 3. 以後 NULL を入れられないようにする。
--
-- 宣言ゼロのプログラムは NewProgram が弾く。DBに NULL が入ると、
-- 読み出しが「保存されたプログラムが不正」で永久に失敗する。
ALTER TABLE program ALTER COLUMN declared SET NOT NULL;
