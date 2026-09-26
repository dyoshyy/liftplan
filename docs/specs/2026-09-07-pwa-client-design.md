# PWA クライアントを React + TypeScript で作り直す

---
date: 2026-09-07
status: 実装済み。画面の数と配信元はその後の判断で上書きされている
---

> **スタック（React + TypeScript + vite-plugin-pwa）の判断はそのまま生きている。**
> 画面は一度1つに絞り（D-120）、履歴を戻した（D-127）。設定は「今日」の末尾に
> 畳んである。配信元はサーバー同居ではなく Cloudflare Workers（D-119）。

## なぜ

いまのクライアントは `app.js` 801行の1ファイルで、型が無い。とりあえず動かすために書いたもので、以下が起きている。

- **型が無い。** サーバーの契約（`weekly_volume`、`last_performances`、`deload_proposal`）が手打ちの文字列で流れる。サーバー側を変えてもクライアントは黙って壊れる
- **分割の単位が無い。** 待ち行列・状態・描画・配線が同じスコープに同居し、1箇所足すたびに他所に触る

当初の計画は PWA を「Android までのつなぎ」と位置づけていた。**この位置づけを取り消す。** Health Connect との連携は捨てがたいが、Android と iOS の両方を作る時間は無い。PWA が本命のクライアントになる。

## スタック

React 19 + TypeScript + Vite + Tailwind CSS v4 + Vitest + pnpm。**追加のランタイム依存はゼロ。**

**入れないもの、とその理由。**

| 入れないもの | 理由 |
|---|---|
| TanStack Router | 画面が3つ（今日・履歴・設定）でネストもパラメータも無い。いまの `showView(name)` 以上のものが要らない |
| TanStack Query | 存在意義がレスポンスのキャッシュと再利用で、**このアプリの「API は絶対にキャッシュしない」と正面衝突する**。`staleTime: 0, gcTime: 0` で全部無効化して使うことになり、それは入れていないのと同じ |
| Dexie / idb | 無くても壊れない。型は TypeScript へ移す時点で手に入るので、Dexie が解決する問題を移植そのものが解決している |
| OpenAPI などのコード生成 | エンドポイントが9つ。生成器を足すほうが重い。契約の型は手書きする |

## 配備：Cloudflare Workers に分離する

当初の計画は「サーバーに同居させる」と決めていた。**この決定を覆す。**

### 同居の根拠のうち、成り立っていなかったもの

- **「成果物が1つなのでバージョンがずれない」は誤り。** Service Worker が殻をプリキャッシュしている以上、インストール済みの PWA は**同居していても古いクライアントで新しいサーバーを叩く**。オフラインで開いたとき、network-first の fetch が返る前は必ずキャッシュの殻が動く。同居は「ずれない」ではなく「ずれる窓が短い」だけ
- **「CDN / ジムの電波」も同じ理由で消える。** 殻は初回以降キャッシュから出るので配信元がどこでも同じ。API の遅延は両方とも同じ。preflight は `Access-Control-Max-Age` でキャッシュされ、送信は待ち行列経由なので画面を止めない。Cloud Run は `--min-instances=0` でコールドスタートが数秒あるが、これも両方に等しく効くので判断材料にならない

### 分離を選んだ理由

他リポジトリと同じ wrangler のデプロイ経路に乗せるため。同居の利点（CORS 不要・デプロイ1箇所）と引き換えに払う。

### ずれたときの被害と、その手当て

`handler.go` に `dec.DisallowUnknownFields()` がある。新しいクライアントがフィールドを1つ足して古いサーバーに送ると **400 が返り、`flush` はそれを再送不能とみなして `rejected` に落とす＝記録が捨てられる。**

手当ては2つ。

1. **クライアントのコードは同じリポジトリの `web/` に置く。** 別リポジトリにすると、契約のずれが本番でしか見えなくなる。同居させれば、API の型を変える PR にクライアントの追従が入り、**ずれがレビューで見える**
2. **デプロイ順序を CI で固定する。** `deploy.yml` の `web` ジョブを `needs: [deploy]` にする。**必ずサーバーが先。** 逆順だと上記の記録破棄が起きる。同居では自動的に守られていた性質なので、分離した以上は明示的に書く

## Go 側の変更

**削除するもの:** `internal/presentation/httpapi/web.go`、`web/` ディレクトリ、`//go:embed`、`staticPaths`、`contentTypes`。allowlist の議論ごと消える。

**追加するもの:** CORS ミドルウェア（`internal/presentation/httpapi/cors.go`）。

- 許可オリジンは環境変数 `ALLOWED_ORIGINS`（カンマ区切り）で**明示した完全一致のみ**。ワイルドカードは使わない。Bearer トークンを持つ API なので、`*` は全サイトから読める状態になる
- **`OPTIONS` は `RequireBearerToken` の外側で完結させる。** ルータが `mux.HandleFunc("POST /api/set-logs", ...)` という Go 1.22 のメソッド付きパターンなので、preflight は認証を抜けても **405 で落ちる**
- `Access-Control-Max-Age` を付けて preflight の往復を減らす
- `Vary: Origin` を付ける。付けないと、途中のキャッシュが別オリジン向けの応答を使い回す

`unauthenticatedPaths` は `staticPaths` を畳み込んでいたので、`/health` だけになる。

## 開発時も本番と同じくクロスオリジンで叩く

Vite の proxy で `/api` を隠さない。`VITE_API_BASE=http://localhost:8080` を使い、Go の `ALLOWED_ORIGINS` に `http://localhost:5173` を許す。

proxy で隠すと、**CORS の設定漏れが本番で初めて出る。** 手元で落ちるほうがよい。

## モジュールの分割

```
web/src/
  api/       fetch と契約の型。唯一 fetch を知る層
  outbox/    IndexedDB・enqueue・flush・冪等ID。React も fetch も知らない
  domain/    日付・セットの整形・区分の日本語。React も fetch も知らない純関数
  storage/   localStorage（トークン・承認済みデロード）
  features/  today / history / settings / setup
  app/       タブ・状態表示・SW 登録・更新の通知
  styles/    Tailwind の @theme トークン
```

**要点は `outbox/` が React も DOM も知らないこと。** ジムで一番壊れてほしくないロジック（再送・冪等ID・修正時の削除→再投入・4xx の破棄）が、DOM もネットワークも立てずに Vitest で単体テストできる。Go 側の「ドメイン層は DB も HTTP も立てずにテストできる」と同じ形にする。

`outbox` は `fetch` そのものではなく **送信関数を引数で受け取る**。これで再送の規則をネットワーク無しで検査できる。

## 保存されているデータは一切触らない

DB 名 `liftplan`、ストア名 `queue` / `rejected`、localStorage のキー `liftplan.token` / `liftplan.queue` / `liftplan.rejected` / `liftplan.deload` を**すべて据え置く**。`adoptLegacyQueue`（古い版が localStorage に残した待ち行列の引き取り）も残す。

移行で記録を落とすのは、このアプリで最もやってはいけないこと。

## Service Worker

**`skipWaiting()` をやめる。** いまは新しい SW が即座に有効になるので、記録シートを開いている最中に画面が差し替わりうる。新版を検知したら状態バーに「更新があります」を出し、**押したときだけ** `SKIP_WAITING` を送って差し替える。

ハッシュ付きの資産をプリキャッシュする必要があるので、precache のリストはビルド時に生成する（`vite-plugin-pwa` の `injectManifest` を使い、SW の中身は手書きのまま保つ）。API のオリジンが別になるため、**「/api をキャッシュしない」は設定ではなく構造上の事実**になる。

`index.html` は `no-cache`、ハッシュ付き資産は `immutable`。Workers の `assets` 設定で指定する。

## テスト

Vitest で `outbox/` と `domain/` を検査する。画面のテストは書かない（Playwright は入れない）。

**必ず検査するもの:**

- 同じIDの再送が二重に積まれないこと
- 4xx は `rejected` へ移して先頭から外すこと（残すと後続が全部詰まる）
- 5xx は残して次の機会に送ること
- 記録の修正が「DELETE を積んでから同一IDで POST」の順になること
- `formatSets` が重量の変わり目で区切ること

liftplan の作法どおり、**テストを先に書いて、期待した理由で赤くなることを確認してから実装する。**

## やらないこと

- Playwright / E2E
- PR ごとのプレビュー配備（Workers のプレビュー URL は毎回変わるので `ALLOWED_ORIGINS` に入れられない。必要になったら suffix 一致に広げる）
- Health Connect との連携（**保留。** PWA からは触れない。将来 Android を作るならそこで扱う）
- 画面の作り直し。いまの見た目・文言・色（IPF プレート色）はそのまま移す

## 実装で踏みやすい穴

**`Outbox.flush` は待ち行列を毎回ストアから読み直す（`head()`）。**手元に配列を
キャッシュして先頭から順に処理する形へ「最適化」すると、送信中に `enqueue`
された分が、ストアからは読み落とされたまま画面には「同期済み」と出る形で消える。
電波の細いジムで連続して記録すると起きる（過去に実際に踏んだ）。この経路を
守るテストはまだ無い。`flush` の実装を変えるときは、先に「flush 中に enqueue
された記録が失われない」テストを書くこと。
