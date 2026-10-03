# liftplan-server

筋トレの進行を自動化するサーバー。実績ログ・週目標・コンディションから、その日のセッション（種目・重量・目標RIR・セット数）を導出する。

設計は `docs/specs/`。ドキュメントの案内は `docs/README.md`。

## 設計の要点

- **未来のセッションは保存しない。** 今日のメニューも来週のメニューも、確定した実績から毎回導出した結果でしかない。だから予定と実績が食い違う状態が原理的に発生しない
- **RIR は止め時の指示であり、レップ数は指示しない。** レップはその日の状態が決めるため、強度が自動でコンディションに追従する
- **減量中の停滞とオーバーリーチによる停滞は、記録だけ見ると同じ形をしている。** 体重と睡眠を取り込むのは、この2つを見分けるため
- **Onion Architecture。** 依存は常に内向き。ドメイン層は標準ライブラリ以外に依存せず、DB も HTTP も立てずにテストできる

## 起動

手元で動かすなら、DB 無し（インメモリ）が一番早い。

```bash
DEV_SESSION_TOKEN=dev-token-0123456789abcdef0123456789ab \
  API_ORIGIN=http://localhost:8080 WEB_ORIGIN=http://localhost:5173 \
  ALLOWED_ORIGINS=http://localhost:5173 \
  GITHUB_CLIENT_ID=dev GITHUB_CLIENT_SECRET=dev \
  GOOGLE_CLIENT_ID=dev GOOGLE_CLIENT_SECRET=dev \
  go run ./cmd/api
```

OAuth の4つは**起動の条件なので値が要るが、起動時に中身は確かめていない**。ログインを通らないなら何でもよい（上の `dev` のままでは本物のログインは通らない）。代わりに `DEV_SESSION_TOKEN` の値がそのままセッショントークンとして通る。

Postgres で動かすなら `DATABASE_URL` を足す。**そのとき `DEV_SESSION_TOKEN` は無視される**ので、入るには本物の OAuth のクライアントIDとシークレットが要る。

```bash
docker compose up -d --wait db
DATABASE_URL='postgres://liftplan:liftplan@127.0.0.1:5433/liftplan' \
  API_ORIGIN=... WEB_ORIGIN=... ALLOWED_ORIGINS=... \
  GITHUB_CLIENT_ID=... GITHUB_CLIENT_SECRET=... \
  GOOGLE_CLIENT_ID=... GOOGLE_CLIENT_SECRET=... \
  go run ./cmd/api
```

`make run` と `make docker-run` は**いまは起動しない**。どちらも廃止した `AUTH_TOKEN` を渡していて、サーバーはそれが残っていると起動を拒む。

| 環境変数 | 既定 | 説明 |
|---|---|---|
| `DATABASE_URL` | なし | Postgres の接続文字列。無ければインメモリで動き、再起動で記録が消える |
| `API_ORIGIN` | **必須** | このサーバー自身のオリジン。OAuth のコールバックURLをここから組むので、認可先に登録したものと一致させる |
| `WEB_ORIGIN` | **必須** | 画面のオリジン。ログイン後の戻り先 |
| `ALLOWED_ORIGINS` | **必須** | 画面のオリジン（カンマ区切り）。ここに無いオリジンからは叩けない |
| `GITHUB_CLIENT_ID` / `GITHUB_CLIENT_SECRET` | **必須** | GitHub の OAuth App |
| `GOOGLE_CLIENT_ID` / `GOOGLE_CLIENT_SECRET` | **必須** | Google の OAuth クライアント |
| `DEV_SESSION_TOKEN` | なし | 開発用のセッションを1件入れる。**`DATABASE_URL` が無いときだけ効く** |
| `PORT` | `8080` | 待ち受けポート |

必須のものが1つでも欠けていたら起動しない。未設定なら素通しする（または黙ってログインを無効にする）挙動にすると、設定漏れがそのまま全公開や「健全に見えるがログインできないサーバー」になるため。起動しないほうが、気づかないまま公開されるよりよい。

**`AUTH_TOKEN` は廃止した。残っていると起動を拒む。**黙って無視すると、「まだトークン認証で動いている」と思ったまま OAuth で公開されるため。

`DEV_SESSION_TOKEN` をフラグではなく「インメモリのときだけ」で守っているのは、フラグだと本番で立った瞬間に固定トークンで入れる穴になるため。インメモリは再起動で記録が消える構成で、そもそも本番では使えない。

`ALLOWED_ORIGINS` にワイルドカードは書けない。Bearer のセッショントークンで守っている API なので、`*` を返すと任意のサイトが利用者のトークン付き要求の結果を読める。

マイグレーションは起動時に自動で流れる。手で流す運用にすると、流し忘れたインスタンスが古いスキーマに書き込む。空のデータベースなら初期プログラム（週3回・同梱の全種目）も入る。

インメモリで動く経路を残しているのは、ドメインの検証を DB 無しで回せる状態を捨てないため。

**種目は利用者ごとの一覧。**共通/個人の2つには分かれていない。プリセット（シードにバイナリ同梱）は、その利用者の行が `user_exercises` に1件も無いときに一度だけコピーする（消した行も「行がある」に数える）。以後はこの表の行がその人の種目一覧そのもので、プリセット由来かどうかで足す・直す・消すの扱いを変えない。

## 認証

GitHub か Google の OAuth でログインする（`GET /auth/{github,google}/start`）。通るとサーバーがセッションを発行し、画面へ `#token=...` で渡す。以後は Bearer で送る。トークンが決めるのは「通してよいか」ではなく**誰の記録か**で、読みも書きもその利用者のものだけに絞られる。

```bash
curl -H 'Authorization: Bearer <セッショントークン>' 'http://localhost:8080/api/sessions?date=2026-08-17'
```

手元では `DEV_SESSION_TOKEN` に渡した値がそのまま使える（インメモリ構成のときだけ）。

認証しないのは `/health` と、ログインの入口である `/auth/*/start`・`/auth/*/callback` だけ（開発用の `/api/dev/*` は後述）。`/health` は Cloud Run の起動プローブが叩けなくなるため。ログアウト（`DELETE /auth/session`）は認証の内側にある。

デプロイ手順は `docs/deploy.md`。

## API

| メソッド | パス | 説明 |
|---|---|---|
| GET | `/health` | ヘルスチェック。保存先への疎通を含む（到達できなければ 503） |
| GET | `/api/sessions?date=YYYY-MM-DD` | その日のセッションを導出する |
| POST | `/api/set-logs` | 実績ログを保存する（冪等） |
| POST | `/api/conditions` | 日次コンディションを保存する（冪等） |
| GET | `/api/program` | プログラム（頻度・週目標・選択種目・伸ばしたい種目・重点種目・分割）を取得する。未設定なら 404 |
| PUT | `/api/program/frequency` | 週の頻度だけを差し替える。週目標も頻度に合わせて置き直される |
| PUT | `/api/program/target` | 週目標だけを差し替える |
| PUT | `/api/program/selected` | 使う種目だけを差し替える |
| PUT | `/api/program/declared` | 伸ばしたい種目だけを差し替える |
| PUT | `/api/program/declared/{id}/reps` | 伸ばしたい種目1つの重い番・軽い番のレップ数だけを差し替える（1〜15）。`GET /api/program` の `declared_reps` に宣言すべてが入る |
| PUT | `/api/program/focus` | 重点種目だけを差し替える。`null` で指定なしに戻す |
| PUT | `/api/program/split` | 分割の周期だけを差し替える。空の配列で分割なしに戻す |
| GET | `/api/split-presets` | 分割のプリセット。`splits` をそのまま `/api/program/split` へ送れる |
| GET | `/api/exercises` | 種目マスタ（IDと日本語名・効き方）。消した種目（`deleted: true`）も含む |
| POST | `/api/exercises` | 種目を足す（201）。足すと使う種目にも入る |
| PUT | `/api/exercises/{id}` | 種目の名前・効き方・刻みを直す（200）。本文は POST と同じ形 |
| DELETE | `/api/exercises/{id}` | 種目を消す（論理削除・204）。プリセット由来かどうかで扱いを変えない。伸ばしたい種目に入っている種目は消せない |
| GET | `/api/set-logs?from=&to=` | 実績と、種目ごとの前回の実績。既定は直近56日 |
| DELETE | `/api/set-logs/{id}` | 打ち間違いの取り消し |
| GET | `/api/stats?from=&to=` | 推定1RMの推移と、今週の週目標の充足 |

### セッション取得の例

```bash
curl -H 'Authorization: Bearer <セッショントークン>' \
  'http://localhost:8080/api/sessions?date=2026-08-17'
```

起動直後（記録が1件も無い状態）の応答。`accessories` は8件返るが、ここでは2件に縮めてある。

```json
{
  "date": "2026-08-17",
  "main": [
    {"exercise_id": "bench", "weight_kg": null, "sets": 3, "target_rir": 1}
  ],
  "variation": [],
  "accessories": [
    {"exercise_id": "back_extension", "weight_kg": null, "sets": 3, "target_rir": 2},
    {"exercise_id": "deficit_deadlift", "weight_kg": null, "sets": 3, "target_rir": 2}
  ]
}
```

`weight_kg` が `null` になるのはバグではない。履歴が足りず重量を推定できない状態で、初回だけ自分で決めて記録する。

### 実績の保存

```bash
curl -X POST http://localhost:8080/api/set-logs \
  -H 'Authorization: Bearer <セッショントークン>' \
  -H 'Content-Type: application/json' \
  -d '{"logs":[{"id":"01J-A","date":"2026-08-17","exercise_id":"bench","weight_kg":85,"reps":9,"rir":2}]}'
```

`id` はクライアントが採番する（ULID を想定）。**同じ ID・同じ内容**の再送は黙って受け入れる（オフラインで記録して後から送るので、タイムアウト後の再送は日常的に起きる）。同じ ID で内容が違う場合は 409 を返す。どちらが正しいか分からないまま上書きすると、推定1RMが静かに動く。

保存は全か無か。1件でも不正なら1件も書かない。

### プログラムの設定

起動時はシードの初期プログラム（週3回・同梱の全種目）が入っているので、設定しなくても使える。

変えるときは、**変えたい1項目だけを、その項目の口へ送る。**プログラムを丸ごと受け取る口は無い（`PUT /api/program` は 405）。丸ごと送らせると、送る側がフィールドを1つ並べ忘れただけで、その設定が黙って消えるため（D-127）。

```bash
curl -X PUT http://localhost:8080/api/program/frequency \
  -H 'Authorization: Bearer <セッショントークン>' \
  -H 'Content-Type: application/json' \
  -d '{"per_week":4}'
```

通れば 204。ボディはどの口も、`GET /api/program` の応答から該当のフィールド1つを抜き出した形。

| パス | ボディ |
|---|---|
| `/api/program/frequency` | `{"per_week":4}` |
| `/api/program/target` | `{"weekly_target":{"CHEST_MID":10}}` |
| `/api/program/selected` | `{"selected_exercises":["bench","squat"]}`。伸ばしたい種目を外す選択は 400（先に `declared` を狭める） |
| `/api/program/declared` | `{"declared_exercises":["bench","squat"]}` |
| `/api/program/declared/{id}/reps` | `{"heavy":8,"light":12}`。範囲外・宣言していない種目は 400 |
| `/api/program/focus` | `{"focus_exercise":"bench"}`（`null` で指定なし） |
| `/api/program/split` | `{"splits":[{"name":"上半身","regions":["CHEST_MID","LAT"]},{"name":"下半身","regions":["QUAD","GLUTE","HAMSTRING"]}]}`（`[]` で分割なし）。伸ばしたい種目が出られる日の無い周期は 400 |

その口のもの以外のフィールドが混ざっていたら 400。受けて捨てると、送った側はそれも変わったと思い込む。

### ステータスコード

| コード | 意味 |
|---|---|
| 400 | 入力が不正（範囲外の値、実在しない種目、必須項目の欠落など） |
| 404 | 未知のパス。`GET /api/program` でプログラムが未設定の場合も 404 |
| 405 | パスは存在するがメソッドが違う |
| 409 | セッション導出時にプログラムが未設定／同じ ID で内容の違うログ |
| 413 | リクエストボディが 1MB を超えた |
| 499 | クライアントが応答を待たずに切断した |
| 504 | 処理が20秒で終わらなかった |
| 500 | 内部エラー。詳細はクライアントに返さずサーバーのログに記録する |
| 503 | 保存先に到達できない。送り直せば通る（500 と区別する） |

`/api/program` の 404 とセッション導出時の 409 は、どちらも「プログラムが未設定」を表す。このバイナリは起動時に初期プログラムを入れるので、通常は起きない。Postgres に差し替えて空のデータベースから始めたときに出る。

すべての POST / PUT は `Content-Type: application/json` を要求する。

## テスト

```bash
make test       # DB を必要としないテスト
make test-db    # Postgres を立てて全テスト
```

ドメイン層のテストは DB も HTTP も必要としない。

`internal/infrastructure/repositorytest` はリポジトリ実装が満たすべき契約
（冪等・同一IDで内容が違えば衝突・全か無か・取得順の安定）のテストスイートで、
**インメモリ実装と Postgres 実装の両方に同じコードを流す**。実装ごとに書き直すと、
片方だけが契約を満たす状態に気づけない。インメモリ実装は失敗しないので、
契約の破れは Postgres で初めて露呈する。

`internal/domain/training/seed/simulation_test.go` は、シードを実際に
セッション生成器へ食わせて頻度1〜4で8週間シミュレートし、週目標の達成率・
種目の出番・セッション長・重量の確定を検証する。シードは「値が入っていること」を
確かめても意味がなく、生成器を通した挙動でしか検証できない。

### シミュレーション画面

通し検証が数字で守るのに対して、**何が起きているかを見る**ための道具。
本番と同じ計画の導出を、模擬ユーザー（実力どおりに記録する本人）で最大12週回し、
宣言種目の重量の推移（処方・記録・その日の実力）と、筋区分ごとの週の充足を出す。
宣言・分割・頻度・曜日・1回の量に加えて、模擬ユーザーの仮定（実力の伸び・
初回の重さ・体重・種目ごとの1RM）も全部変えられる。

設定は URL に載る（`/api/dev/simulate` と同じキー）。URL を開けばその設定で
結果が出て、先頭の「要約」が数字を文字で持つ。Claude から確かめるときの手順は
`.claude/skills/dev-simulation/`（API を直接叩く経路）。

本番にもある。画面のオリジンの `/dev.html`（ログイン済みの端末で開く）。
手元だけに置いていたが、設定を変えるかどうかを考えるのはたいていジムの
あとで、そこに開発機が無い。

```bash
# サーバー。手元ではインメモリなので、DEV_SESSION_TOKEN でログイン済みの
# 状態を作る（この変数は DATABASE_URL があるときは効かない）。
#
# OAuth の設定は起動の条件なので値が要るが、この経路は通らないので
# 中身は何でもよい。
DEV_SESSION_TOKEN=test-token-0123456789abcdef0123456789abcdef \
  API_ORIGIN=http://localhost:8080 WEB_ORIGIN=http://localhost:5173 \
  ALLOWED_ORIGINS=http://localhost:5173 \
  GITHUB_CLIENT_ID=dev GITHUB_CLIENT_SECRET=dev \
  GOOGLE_CLIENT_ID=dev GOOGLE_CLIENT_SECRET=dev \
  PORT=8080 go run ./cmd/api

# 画面
cd web && VITE_API_BASE=http://localhost:8080 pnpm dev

# 1. http://localhost:5173/#token=test-token-0123456789abcdef0123456789abcdef
#    を一度開く（トークンを localStorage に入れる。本物のコールバックと同じ形）
# 2. http://localhost:5173/dev.html
```

口は**認証の内側**にある。捏造した設定で計画を作るだけで保存先も記録も
触らないが、週7回×12週の導出は CPU を使うので、誰でも叩ける状態では置かない。

## 設計の判断記録

設計ごとの判断とその根拠は `docs/specs/` にある。骨格が変わった判断は、
今日の計画はその日の始まりに確定する（`internal/domain/training/planning/session_planner.go`、
`TestSessionPlanner_PlanIsFixedForTheWholeDay`）、軸は枠ではなく宣言
（`docs/specs/2026-09-06-training-goals-design.md`）、推定1RMは種目ごとに持つ
（`internal/domain/training/seed/exercises.go`）、42日より古い記録からは推定しない
（`internal/domain/training/planning/one_rep_max_estimator.go`）、週目標は頻度で
スケールする（`internal/domain/training/seed/weekly_target.go`）あたり。
