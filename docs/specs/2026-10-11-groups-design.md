---
date: 2026-10-11
status: 実装済み（PR 1 ドメイン・保存 #287・PR 2 API #288・PR 3 画面 #290・PR 4 招待から参加）
---

# グループで記録を見せ合う

**日付**：2026-10-11

## 何のためか

**続ける動機。**仲間が今週やっているのが見えて、自分もサボれなくなること。
重量を比べ合うためではない。

だから見せるのは**日と種目名まで**。重量・レップ・RIR は見せない。動機には
「やったかどうか」で足り、重量が見えることへの抵抗を持ち込まずに済む。

## 決めたこと

| 論点 | 決定 | 理由 |
|---|---|---|
| 見せる範囲 | 日と種目名 | 目的（続ける動機）に足りる。重量は見せない |
| グループの数 | 1人が複数に入れる | 本人の判断（ジム仲間と同僚を分ける） |
| 表示名 | 参加するときにグループごとに入れる。1〜20文字、一意にしない | 場所ごとに呼ばれ方が違う。同名がいても壊れない |
| 画面 | メンバーごとの今週（月〜日）の一覧。日を押すとその日の種目 | 「自分だけ空いている」が一目で分かる |
| 管理 | リンクはグループに1本。メンバーなら誰でも共有・作り直し（古いリンクは無効）。抜けるのは本人だけ。管理者・追い出し・期限は無し | 必要になるまで作らない |
| 見える相手 | いま同じグループにいるメンバーだけ。抜けたら互いに見えない | — |
| 参加前の記録 | 見える | 画面は今週だけなので実害が無い |
| 最後の1人が抜けたら | グループを消す | 誰も見られないグループを残さない |
| 前の週 | 見られない | 欲しくなってから足す |

## 他人の記録を読む壁をどこに開けるか

既存のリポジトリは「他人の記録は見えない」を契約にしている
（`postgres/cross_user_test.go`）。共有はこの壁に穴を開ける話で、穴は
**決めた形の1か所**に限る。

**判断はドメイン、取得はインフラ。**「誰が誰の記録を見てよいか」は業務の
規則で、変わりうる（抜けた人・参加前の記録）。SQL に書くと規則がインフラに
住むので、`group.Group.VisibleTo` だけが決める。

```go
// VisibleTo は viewer がメンバーなら、見せてよいメンバーを返す。違えば ErrNotMember。
func (g *Group) VisibleTo(viewer account.UserID) (Audience, error)

// Audience は非公開フィールドだけを持つ。VisibleTo を通さずに中身のある値は作れない。
type Audience struct{ members []Member }

type ActivityReader interface {
    Activity(ctx context.Context, a Audience, from, to training.Date) ([]Activity, error)
}
```

- 読み口は生の `[]UserID` を受け取らない。ユースケースが確認を忘れると
  コンパイルが通らない
- `group.Audience{}`（ゼロ値）は外から書けるが、誰も含まないので何も読めない
- `Activity` に重量のフィールドが無い。見せる範囲を型で守る
- 確認してから読むまでの間に抜けた人の分が1回だけ見えうる。日と種目名だけ
  なので許容する

既存の `WHERE user_id = $1` は「誰のデータか」で引き分けているだけで、
規則ではない。グループの見える範囲はそれと違って変わりうるので、ドメインに置く。

## 保存

```sql
CREATE TABLE groups (
    id           uuid        PRIMARY KEY,
    name         text        NOT NULL,
    invite_token text        NOT NULL UNIQUE,
    created_at   timestamptz NOT NULL
);
CREATE TABLE group_members (
    group_id     uuid        NOT NULL REFERENCES groups (id) ON DELETE CASCADE,
    user_id      uuid        NOT NULL,
    display_name text        NOT NULL,
    joined_at    timestamptz NOT NULL,
    PRIMARY KEY (group_id, user_id)
);
CREATE INDEX group_members_user_idx ON group_members (user_id);
```

- **招待トークンは平文。**メンバーはいつでも同じリンクを見せ直す。ハッシュに
  すると共有のたびに作り直すことになる。DB が漏れて失うのは「入れば日と種目名が
  見える権利」で、それは `set_logs` と一緒に既に漏れている。128bit の乱数にする
- `user_id` に外部キーを張らない（0008 と同じ。参照先の users 表が無い）
- 種目名は**その人の** `user_exercises` から引く。自作種目（`u-…`）も、論理削除
  した種目も名前が出る

仲間の記録の SQL は重量を SELECT しない。

```sql
SELECT DISTINCT l.user_id, l.performed_on, e.name
  FROM set_logs l
  JOIN user_exercises e ON e.user_id = l.user_id AND e.id = l.exercise_id
 WHERE l.user_id = ANY($1) AND l.performed_on BETWEEN $2 AND $3
```

### 最後の1人が抜けたとき

`Group.Leave` のあとに `IsEmpty` なら、ユースケースが `DeleteIfEmpty` を呼ぶ。
消すと決めるのはドメインで、保存先は**消す瞬間にまだ空か**だけを確かめる
（`NOT EXISTS` を同じ1文に入れる）。

確かめないと、最後の1人が抜けると判断してから消すまでの間に誰かが参加した
とき、その人の行が `ON DELETE CASCADE` で一緒に消える。参加できたのに入って
いない人ができる。これは業務の規則ではなく、同時に起きた変更どうしを
踏まないための確認なので、SQL に置く。

逆に2人が同時に抜けると、どちらも相手が残っていると見て空のグループが
残りうる。メンバーがいないので誰にも見えず、リンクから入れば普通の
グループとして使えるので、壊れない。

## API

全部ログイン必須。

| メソッドとパス | 何をするか |
|---|---|
| `GET /api/groups` | 自分のグループの一覧 |
| `POST /api/groups` `{name, display_name}` | 作って最初のメンバーになる |
| `GET /api/groups/{id}/week?date=` | その日を含む週の、メンバーごとの日と種目名。招待トークンも返す |
| `POST /api/groups/{id}/invite` | 招待リンクを作り直す |
| `DELETE /api/groups/{id}/members/me` | 抜ける |
| `GET /api/invites/{token}` | リンク先のグループ名・人数・もう入っているか |
| `POST /api/invites/{token}/join` `{display_name}` | 参加する |

- 週は画面が `date` で渡す（端末の今日）
- 参加は冪等。既にメンバーなら何も変えず（表示名も変えず）グループの ID を返す
- メンバーでない人の `/api/groups/{id}/…` は **404**。403 だとグループの存在が分かる
- 無効になったリンクも 404

## 招待リンクの流れ

リンクは `<画面のオリジン>/?invite=<token>`。画面にはルーターが無いので
クエリ1つで受ける。

1. 起動時に `invite` を読み、localStorage に移して URL から消す
   （`history.replaceState`）。ログインの往復で落ちず、履歴にも残らない
2. 未ログインなら普段どおりログインする。認証の戻り先は固定のまま
   （open redirect の対策を崩さない）
3. ログイン済みで招待があれば参加の画面を出す
4. 参加かキャンセルで localStorage から消す。404 なら「このリンクは使えない」

グループの画面は読むだけなので outbox に乗せない。オフラインなら
「つながったら見られます」とだけ出す。

## 画面

下のナビの3つ目に「仲間」を足す。

- グループが無ければ「グループを作る」（グループ名・自分の名前）
- 上に切り替え（最後に見たものは端末に覚える）と、共有・作り直し・抜ける
- メンバー × 月〜日の格子。自分が先頭。● を押すとその日の種目
- 共有は Web Share API、無ければクリップボード
- 確認は画面内に出す（`window.confirm` は使わない）
- 判断（`.ts`）／手順（`use*.ts`）／描画（`.tsx`）に割る

## PR の割り方

1. ドメイン（`group`）＋マイグレーション＋リポジトリ（Postgres・インメモリ）。誰も呼ばない
2. ユースケースと API
3. 画面：「仲間」タブ・作る・週の格子・共有・作り直し・抜ける
4. 画面：招待リンクから参加する流れ

## 検収

- `domain/group`：`VisibleTo` がメンバー以外に `ErrNotMember`。参加の冪等。最後の1人で空。名前の検証。
  変異：`VisibleTo` が誰にでも返すと赤
- リポジトリ（Postgres・インメモリで同じケース名）：読めるのは Audience の人だけ。種目名は本人の一覧から
  （自作・論理削除も）。作り直した後の古いトークンで引けない。変異：`ANY($1)` を外すと赤
- httpapi：メンバー以外に 404。参加のやり直しで同じ結果
- 旧版で DB を作ってから新版を当てる
- vitest：週の格子、招待の localStorage の扱い。`web/scripts/groups-check.mjs` で配線
