-- グループ。記録（日と種目名）を見せ合う仲間の集まり
-- （docs/specs/2026-10-11-groups-design.md）。
--
-- 招待トークンは平文で持つ。セッションのトークン（0008）はハッシュにしたが、
-- 招待リンクはメンバーがいつでも見せ直すので、元の値が要る。漏れて失うのは
-- 「入れば日と種目名が見える権利」で、その中身は set_logs と一緒に既に漏れている。
-- UNIQUE は引くための索引を兼ねる。
CREATE TABLE groups (
    id           uuid        PRIMARY KEY,
    name         text        NOT NULL,
    invite_token text        NOT NULL UNIQUE,
    created_at   timestamptz NOT NULL
);

-- メンバー。表示名はグループごとに持つ（場所ごとに呼ばれ方が違う）。
--
-- 主キーが (group_id, user_id) なので、同じ人が同じグループに二度入らない。
-- 参加の二度押しは ON CONFLICT DO NOTHING で受ける。
--
-- user_id に外部キーを張らないのは 0008 と同じ（参照先の users 表が無い）。
-- group_id には張る。グループを消したらメンバーの行も消える。
CREATE TABLE group_members (
    group_id     uuid        NOT NULL REFERENCES groups (id) ON DELETE CASCADE,
    user_id      uuid        NOT NULL,
    display_name text        NOT NULL,
    joined_at    timestamptz NOT NULL,
    PRIMARY KEY (group_id, user_id)
);

-- 自分のグループの一覧（ListByMember）が利用者で引く。主キーの先頭は
-- group_id なので、それだけでは使えない。
CREATE INDEX group_members_user_idx ON group_members (user_id);
