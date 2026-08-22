# 第6部: 認証とデプロイ

> **エージェント向け:** 1タスク1PRで実装する。各PRの後に敵対的検証を行い、穴を潰してから次へ進む。

**ゴール:** 自分が実際に使える状態にする。公開された URL があり、自分だけが叩ける。

**なぜ今これなのか:** サーバーは完成しているが、まだ1回も使われていない。スロットの強度（0.81 / 0.88 / 0.76）も週目標も**シミュレーションで検証しただけ**で、本人の体では検証されていない。定数を直す唯一の方法は実際に使うことなので、使い始めるのが遅れるほど、間違った数字の上に機能が積み上がる。

**構成:** Cloud Run（コンテナ）+ Neon（Postgres）。どちらもスケールゼロで、使わない間はほぼ課金されない。

## 判断: なぜ Cloudflare Workers ではないのか

Workers は V8 isolate なので、Go を載せるなら WASM にする必要がある。ところが Go の WASM ターゲット（`GOOS=js` / `wasip1`）では **`net` パッケージが使えない**。pgx は `net.Conn` の上に立っているので、Postgres へ繋ぐ手段が無い。Hyperdrive も Worker のバインディングで、`nodejs_compat` 前提の JS ドライバ向けなので解決にならない。

Workers に載せるにはサーバーを TypeScript か Rust で書き直すことになる。ドメイン層と判断記録を捨てる価値は無い。

Cloudflare Containers なら Dockerfile を載せられるが、beta で、前段に TypeScript の Worker を書く必要があり、コールドスタートが 2〜3秒。Cloud Run なら同じことが TS の層なしでできる。

## 全体の制約

- 認証は**単一ユーザー向けの最小構成**。ユーザーという概念をドメインに持ち込まない
- `/health` は認証しない。Cloud Run の起動プローブが叩けなくなる
- **トークンが未設定なら起動しない**。設定を忘れたまま公開されるほうが、起動しないより悪い

---

## Task 30: Bearer トークン認証

**Files:**
- Create: `internal/presentation/httpapi/auth.go`
- Test: `internal/presentation/httpapi/auth_test.go`
- Modify: `cmd/api/main.go`

**Produces:**
- `func RequireBearerToken(token string) func(http.Handler) http.Handler`

**判断:**

- **長いランダムトークンの Bearer 認証**。単一ユーザーの個人アプリなら十分で、Android 側は安全なストレージに入れるだけ。OAuth へはミドルウェアの差し替えで移れる
- **presentation 層に置く**。HTTP の関心事であって、ドメインにもユースケースにもユーザーという概念を持ち込まない（D-066 で予想したとおりの場所）
- **比較は定数時間**（`crypto/subtle`）。素朴な `==` は応答時間からトークンを1バイトずつ推測できる
- **トークンが空なら起動を止める**。「未設定なら認証しない」にすると、環境変数の設定漏れがそのまま全公開になる
- **401 に `WWW-Authenticate: Bearer`** を付ける。クライアントが「認証が要る」と「壊れている」を区別できる

---

## Task 31: Dockerfile と Cloud Run 設定

**Files:**
- Create: `Dockerfile`
- Create: `.dockerignore`
- Modify: `Makefile` / `README.md`

**判断:**

- **マルチステージ + distroless**。ビルド環境をイメージに残さない。実行ユーザーは非 root
- **`CGO_ENABLED=0`**。静的リンクにして、ベースイメージの libc に依存しない
- **`PORT` を読む**。Cloud Run はポートを環境変数で渡してくる（実装済み）
- **マイグレーションは起動時のまま**。Cloud Run はスケールゼロなので毎回のコールドスタートで走るが、アドバイザリロックと適用済み判定があるので数クエリで済む

---

## Task 32: Neon と Cloud Run へのデプロイ

**Files:**
- Create: `docs/deploy.md`

Neon プロジェクトと Google Cloud プロジェクトは**本人のアカウントに作る**。手順を書き、実行はコマンドとして渡す。

**判断:**

- **Neon は直接接続の文字列を使う**（`-pooler` ではない）。理由は D-071。当初「prepared statement と相性が悪い」と書いていたが、これは実際に流して**再現しなかった**。本当の理由はマイグレーションのアドバイザリロック
- **秘密は Secret Manager**。環境変数に直書きすると、コンソールの表示にも `gcloud run services describe` にも出る
- **`--min-instances=0`**。使わない間は課金しない。コールドスタートは数秒だが、ジムで最初に開くとき以外は効かない
- **リージョンは `asia-southeast1`（シンガポール）**。Neon に東京リージョンが無く、最寄りがシンガポールになるため。DB に近づけるほうが速い（D-071）
