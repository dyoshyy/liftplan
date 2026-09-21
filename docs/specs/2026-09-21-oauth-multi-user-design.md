---
date: 2026-09-21
status: 実装中（PR 1 マージ済み。2・3・5・6・9 レビュー待ち）
---

# OAuth ログインとマルチユーザー化

GitHub / Google でログインできるようにし、記録をユーザーごとに分ける。

## いまどうなっているか

- 認証は `AUTH_TOKEN` 1本の Bearer（D-068/D-069）。画面は Setup でトークンを手入力し
  `localStorage` に置く。
- **ユーザーという概念がどこにも無い。**`program` テーブルは `id boolean PRIMARY KEY
  DEFAULT true CHECK (id)` で1行に固定。`set_logs` `daily_conditions` にも所有者の列が無い。
- リポジトリの口（`setlog.Reader/Writer`、`condition.*`、`program.*` の3種）は
  `ctx` しか取らない。ユースケース15本とクエリ2本（`history` `stats`）も同様。
  種目マスタ（`exercise.Reader`）はシード由来の静的なマスタなので、ユーザーに
  依らないまま残す。

D-068 は「OAuth へはミドルウェアの差し替えで移れる」「マルチユーザーに開くときは
スキーマに `user_id` を足す作業とセット」と書いている。今回はその両方をやる。

## PR の割り方

9本。**1本に1つの判断**、動作の変更と機械的な移動は混ぜない。

### 所有者を通す（外から見た動作は変わらない）

| | 中身 | 状態 |
|---|---|---|
| **1** | `internal/domain/account/` に `UserID` 値オブジェクトと既定ユーザーの定数 | #96 マージ済み |
| **2** | マイグレーション `0007`。`user_id` を足し、既存行を既定ユーザーで埋め、主キーを `(user_id, ...)` に変える | #98 |
| **3** | リポジトリの口3種に `UserID` を貫通。両実装と契約テスト。`0009` で足場を落とす | #101 |
| **4** | ユースケース15本・クエリ2本・`httpapi` に貫通。ミドルウェアが既定ユーザーを入れる | 作業中 |

### 認証を差し替える

| | 中身 | 状態 |
|---|---|---|
| **5** | `Account` `Session` のドメインと口、マイグレーション `0008` | #100 |
| **6** | `internal/infrastructure/oauth/`。GitHub と Google のコード交換と identity 取得 | #97 |
| **7** | `SignIn` ユースケース。Account を引く／無ければ作る、初期プログラム、セッション発行 | 作業中 |
| **8** | `/auth/*` のルートとミドルウェアの差し替え、`cmd` の配線。`AUTH_TOKEN` 廃止 | 未着手 |

### 画面

| | 中身 | 状態 |
|---|---|---|
| **9** | フラグメントの取り込み、Setup をログインボタン2つに、ログアウト | #99（draft） |

**2 を独立させた理由。**`user_id` の列に `DEFAULT` を付けておけば、既存の SQL 文が1行も変わらずに通る。だから「スキーマの移行」と「口の使い方の変更」を混ぜずに済む。足場（`DEFAULT` と旧い `UNIQUE` と `program.id`）は 3 の `0009` で落とす。

**やってみて分かったこと。**`0007` は `DEFAULT` を付けるだけでは足りなかった。Go の `INSERT` が `ON CONFLICT (id)` を使っており、**Postgres は指定した列そのものに一意索引が無いとこの文を実行時に拒否する**。主キーを複合にした瞬間に `(id)` 単独の索引が消えるので、旧い形の `UNIQUE` を移行中だけ残す必要があった。同じ理由で `program.id` も 2 では落とせない。

**8 と 9 は同じ窓でデプロイする。**8 だけ出すと、端末の古いトークンが 401 になり Setup に戻されるのに、その Setup は誰も作れないトークンを要求する。溜まった記録は待ち行列に残るので実害は「その間使えない」だけだが、順序は決めておく。

**1〜4 は途中でデプロイしてよい。**外から見た動作が変わらないので、「誰でもログインできて全員が同じデータを見る」中間状態は作れない。逆順にするとそれが作れてしまう。

**9 が 8 に要求していること。**`web/scripts/*-check.mjs` は固定のトークン（`AUTH_TOKEN`）でログイン済みの状態を作る。8 が `AUTH_TOKEN` を廃止した瞬間に5本とも 401 になり、画面の検収が止まる。開発用の抜け道（セッションを1件シードする等）を 8 で用意すること。

## 決めたこと

### UserID は引数で渡す。context に入れない

`context` に入れると、リポジトリの口を見ても「誰のデータか」が読めない。
渡し忘れがコンパイルで落ちず、**実行時に他人のデータを返す**形で出る。
引数にすれば、口の形がそのまま「ユーザーごとに分かれている」と言う。

貫通する箇所は多い（リポジトリ3種・ユースケース15本・クエリ2本）が、これは
機械的な変更で、判断は入っていない。

**例外は1つだけ。**ミドルウェアからハンドラへ渡す手段は `r.Context()` しか無い。
`httpapi` の中だけは context で運び、**ハンドラが取り出したらそこで終わり**にする。
その先（ユースケース・リポジトリ）へ context のまま流さない。

### 主キーに所有者を含める

`set_logs.id` はクライアントが採番する。`user_id` を列に足すだけで主キーを `id` の
ままにすると、**他人のIDと衝突したときに漏れる**。B が A のIDで違う内容を送れば
`ErrConflictingSetLog`（他人の記録の存在が分かる）、B が A のIDを消せば A の記録が
消える。

- `set_logs` → `PRIMARY KEY (user_id, id)`
- `daily_conditions` → `PRIMARY KEY (user_id, date)`
- `program` → `id boolean` を捨てて `user_id` を主キーに

リポジトリの契約テストに次を足す（インメモリ・Postgres の両方）。

- B が A と同じIDで違う内容を保存できる（衝突にならない）
- B が A のIDを消しても A の記録は残る
- A の `FindAll` に B の記録が出ない

**変異で検収する。**`DELETE` の `WHERE` から `user_id =` を外して赤くなることを見る。

### 既存の記録は既定ユーザーに寄せ、あとから本人のアカウントに結ぶ

PR A は既存の全行を固定の UserID（マイグレーションとコードに定数で置く UUID）に
寄せる。PR B は初回ログインで**新しい** Account を作るので、何もしないと
**本人がログインした瞬間、これまでの記録が誰のものでもなくなる。**

Neon のコンソールから SQL を流せるので、初回ログインの前に1回だけ手で結ぶ。

```sql
INSERT INTO accounts (provider, subject, user_id)
VALUES ('github', '<自分の GitHub の数値ID>', '<既定ユーザーの UUID>');
```

「最初にログインした人が既存の記録を引き継ぐ」にはしない。デプロイ直後に
見知らぬ人が先にログインすると記録を持っていかれる。

### セッションはトークンを DB に置く。Cookie にしない

画面は `*.workers.dev`、API は Cloud Run で**別オリジン**。API が出す Cookie は
SPA から見てサードパーティで、Safari は落とす。いまの `Authorization: Bearer` と
`localStorage` の形を保つ。

`sessions` 表には**トークンそのものではなく SHA-256 のハッシュ**を置く。DB が
漏れてもセッションは漏れない。ハッシュで引けるので定数時間比較も要らない。

期限は **90日**。ジムで毎回ログインし直すアプリに価値は無い。ログアウトは
`DELETE /auth/session` で、その場で行を消す。

### コールバックからSPAへの受け渡しはフラグメント

`https://<web>/#token=...` にリダイレクトする。クエリだと Referer とアクセスログに
残る。SPA は起動時に `location.hash` を読み、`setToken` して `history.replaceState`
で消す。

**戻り先は `WEB_ORIGIN` 1つを環境変数で持つ。**`redirect_uri` をパラメータで
受け取ると open redirect になる。画面が1つしか無いうちは、候補から選ばせる仕組みも
要らない。必要になってから足す。

### state は API オリジンの Cookie に置く

`/start` → プロバイダ → `/callback` は全て API オリジンへのトップレベル遷移なので
ファーストパーティ。`SameSite=Lax` で生き残る。

### Google は id_token を自前で検証せず userinfo を叩く

JWT の検証（JWKS の取得・鍵の回転・aud/iss/exp）を自前で持つと、間違えたときに
**通ってはいけないトークンが通る**。access token で userinfo を引けば、検証は
プロバイダ側にある。依存は `golang.org/x/oauth2` 1つで済む。

### 未設定なら起動しない

D-068/D-119 と同じ。クライアントID・シークレット・`ALLOWED_ORIGINS` のどれかが
欠けていたら `buildHandler` が失敗する。

## 層の割り当て

| 何 | どこ |
|---|---|
| `UserID` 値オブジェクト | `internal/domain/account/`（training には置かない。同一性は training の集約ではない） |
| `Account` 集約（provider・subject・UserID） | `internal/domain/account/` |
| `Session`（トークンハッシュ・UserID・期限） | 同上 |
| 初回ログインの受け入れ（Account 作成・初期プログラム・セッション発行） | `internal/application/usecase/sign_in.go` |
| プロバイダとの通信・コード交換 | `internal/infrastructure/oauth/` |
| `/auth/*` のハンドラ | `internal/presentation/httpapi/` |
| 配線・環境変数 | `cmd/api/main.go` |

## 検収

- PR A：全テストが緑のまま。加えて上の3つの契約テスト（インメモリと Postgres の両方）
- PR A のマイグレーション：`main` を別の worktree に出して**旧版でDBを作ってから**
  新版を当てる。`git stash` は未追跡ファイルを退避しないので、新しい `0007` が
  旧版の実行にも混ざる（CLAUDE.md の「一度これで検証を誤った」がこれ）
- PR B：偽プロバイダ（`httptest`）で state 不一致 → 400、コード交換失敗 → 502、
  成功 → セッションが1件でき、フラグメント付きでリダイレクトされる
- PR C：`web/scripts/*-check.mjs`

## 人がやること（コードでは済まない）

- GitHub OAuth App を作り、コールバック URL を登録
- Google Cloud で OAuth クライアントを作り、同じく登録
- Cloud Run に環境変数を設定し、`AUTH_TOKEN` を外す（`WEB_ORIGIN` を足す）
- **初回ログインの前に**、上の `accounts` の1行を Neon の SQL エディタから流す

## 分かっていないリスク

インストール済み PWA（iOS）から `accounts.google.com` へ遷移すると Safari に
飛ばされることがあり、Safari と PWA は `localStorage` を共有しない。トークンが
別のコンテキストに落ちる可能性がある。**実機で確かめる。**踏んだ場合の逃げ道は、
フラグメントではなく PWA 側が短命のコードを1回交換しに行く形。
