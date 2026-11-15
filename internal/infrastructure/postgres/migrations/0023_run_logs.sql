-- ランの記録。計画の導出には使わない（有酸素の日の配置は今日から先しか
-- 見ない）。記録として残すだけ。
--
-- 主キーに所有者を含めるのは set_logs と同じ理由（0007）。id はクライアントが
-- 採番するので、id 単独だと他人と同じIDを引いたときに他人の行へ手が届く。
--
-- 距離はメートル、時間は秒の整数で持つ。浮動小数だと、同じ内容の再送を
-- 比べたときに丸めの差で衝突扱いになる。範囲の検証はドメインの
-- コンストラクタが持つ。ここで CHECK を重ねると、上限を変えるたびに
-- マイグレーションが要る。
CREATE TABLE IF NOT EXISTS run_logs (
    user_id          uuid    NOT NULL,
    id               text    NOT NULL,
    performed_on     date    NOT NULL,
    distance_m       integer NOT NULL,
    duration_seconds integer NOT NULL,
    PRIMARY KEY (user_id, id)
);
