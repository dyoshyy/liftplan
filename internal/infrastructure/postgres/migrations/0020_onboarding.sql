-- 初回の案内を済ませた利用者。行が無い＝まだ。
--
-- program の列にしないのは、Program が訓練の設定を持つ集約で、画面の
-- 進み具合は計画の導出に一切効かないため。混ぜると、計画を読む人が
-- 「この値はどこで効くのか」を探すことになる。
CREATE TABLE IF NOT EXISTS onboarding (
    user_id      uuid        PRIMARY KEY,
    completed_at timestamptz NOT NULL
);

-- 既存の利用者は済んだものとして扱う。program は初回ログインで必ず作られる
-- ので、program がある＝この版より前にログインした利用者。
INSERT INTO onboarding (user_id, completed_at)
SELECT user_id, now() FROM program
ON CONFLICT (user_id) DO NOTHING;
