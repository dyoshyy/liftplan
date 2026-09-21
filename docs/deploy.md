# デプロイ

Cloud Run（コンテナ）+ Neon（Postgres）。どちらもスケールゼロで、使わない間はほぼ課金されない。

## なぜこの構成か

- **Cloud Run**: Dockerfile をそのまま載せられる。TypeScript の層もフレームワークも要らない
- **リージョンはシンガポール**（`asia-southeast1`）。Neon に東京が無いので、DB と同居させる。1リクエストで DB を4本叩くため、ユーザーに近づけるより DB に近づけるほうが速い（東京 CR + シンガポール DB は約290ms、シンガポール同居は約74ms）
- **Neon**: Postgres そのもの。開発中の Docker Postgres と接続文字列の形が同じで、検証したものがそのまま動く

Cloudflare Workers を選ばなかった理由は `docs/decisions.md` の D-067 に書いた。要点は、Go の WASM ターゲットで `net` パッケージが使えず pgx が動かないこと。

## 1. Neon のプロジェクトを作る

コンソール（https://console.neon.tech）で作る。**Neon に東京リージョンは無い**ので、最寄りは `AWS ap-southeast-1 (Singapore)`。

接続文字列は**プーラー無しのほう**を使う。コンソールが既定で見せるのは `-pooler` 付きなので、ホスト名から `-pooler` を外す。

```
postgres://<user>:<password>@ep-xxxx.ap-southeast-1.aws.neon.tech/neondb?sslmode=require
                                    ↑ -pooler が付いていないこと
```

理由はマイグレーションが**セッションレベルのアドバイザリロック**を使っているから。トランザクションプーリングはバックエンドの固定を保証しないので、ロックの前提が崩れる（D-071）。

pooler でもアプリ自体は動く。ただしテストは `search_path` でスキーマを分離しており、pooler はそれを拒否する。**本番とテストで同じ経路を通す**ためにも直接接続で揃える。

## 2. 認証トークンを作る

```bash
openssl rand -hex 32
```

32文字未満だとサーバーが起動しない。

## 3. GCP プロジェクトと課金

```bash
gcloud projects create liftplan-xxxxx --name=liftplan
gcloud config set project liftplan-xxxxx
```

課金アカウントを紐づける。**Cloud Run は無料枠が大きい（月200万リクエスト）ので実質 0 円**だが、有効な請求先の紐付け自体は必須。

```bash
gcloud billing accounts list                      # OPEN が True のものを使う
gcloud billing projects link liftplan-xxxxx --billing-account=XXXXXX-XXXXXX-XXXXXX
```

請求先が閉じている場合は、コンソール（https://console.cloud.google.com/billing）で
支払い方法を登録して開き直す。CLI からはできない。

## 4. 秘密を Secret Manager に入れる

環境変数に直書きしない。`gcloud run services describe` にもコンソールにも出てしまう。

```bash
gcloud services enable run.googleapis.com secretmanager.googleapis.com \
  artifactregistry.googleapis.com cloudbuild.googleapis.com

printf '%s' '<Neon の接続文字列>' | \
  gcloud secrets create liftplan-database-url --data-file=-
printf '%s' '<生成したトークン>' | \
  gcloud secrets create liftplan-auth-token --data-file=-
```

## 5. デプロイ

```bash
gcloud run deploy liftplan-server \
  --source . \
  --region asia-southeast1 \
  --min-instances=0 \
  --max-instances=2 \
  --cpu=1 --memory=512Mi \
  --set-secrets=DATABASE_URL=liftplan-database-url:latest,GITHUB_CLIENT_SECRET=liftplan-github-client-secret:latest,GOOGLE_CLIENT_SECRET=liftplan-google-client-secret:latest \
  --set-env-vars="ALLOWED_ORIGINS=<画面のオリジン>,WEB_ORIGIN=<画面のオリジン>,API_ORIGIN=<このサービスのURL>,GITHUB_CLIENT_ID=<...>,GOOGLE_CLIENT_ID=<...>" \
  --allow-unauthenticated
```

`--allow-unauthenticated` は **Cloud Run 側の IAM 認証を切る**という意味で、アプリの認証は別に効いている。ここを閉じると Google のアカウントが要るようになり、Android から叩けない。

`--max-instances=2` にしているのは、単一ユーザーで台数が増える理由が無いのと、Neon の接続数を使い切らないため。1インスタンスあたり最大8接続を張る。

## 6. 確認

```bash
URL=$(gcloud run services describe liftplan-server \
  --region asia-southeast1 --format='value(status.url)')

curl -s "$URL/health"                      # {"status":"ok"}
curl -s -o /dev/null -w '%{http_code}\n' "$URL/api/program"   # 401
curl -s -H "Authorization: Bearer <トークン>" "$URL/api/program"
```

マイグレーションは起動時に自動で流れる。空のデータベースなら初期プログラムも入る。

## リリース履歴をたどる

```bash
make releases
```

```
-> liftplan-server-1b5d76b-12   2026-08-22 15:04  1b5d76b   feat: 自動デプロイ
   liftplan-server-a33a58a-11   2026-08-22 14:56  a33a58a   fix: ヘルスチェックの経路
```

**Cloud Run のリビジョンがリリース台帳**になっている。Git のタグは打たない。
二重管理になるだけで、分かることが増えないため。

情報の在りかは3つで、**イメージのタグがコミットSHAなので全部つながる**。

| 知りたいこと | 引き方 |
|---|---|
| 何が今動いているか | `gcloud run services describe ... --format='value(spec.template.spec.containers[0].image)'` → タグ部分がコミットSHA |
| いつ何が出たか | `make releases` |
| なぜ出たか（誰が押したか） | `gh run list --workflow=Deploy` / `gh run view <ID> --log` |

リビジョン名に短縮SHAが入っているので、一覧を見るだけでどのコードが動いて
いるか分かる。名前に実行番号を足しているのは、同じコミットを出し直したときに
名前が衝突してデプロイが失敗するため（再実行やロールバックで起きる）。

`commit` と `run-id` はリビジョンのラベルにも入っている。

```bash
gcloud run revisions describe <リビジョン名> --region=asia-southeast1 \
  --project=liftplan-85309 --format='value(metadata.labels)'
```

## GitHub 側に入れておくもの

`deploy.yml` はこれらが無いと**途中で止まる**。既定値に落ちない作りにしてあるので、
設定漏れは黙って通らずジョブの失敗として出る。

| 種類 | 名前 | 中身 |
|---|---|---|
| リポジトリ変数 | `ALLOWED_ORIGINS` | 画面のオリジン。Workers の URL（例 `https://liftplan-web.<サブドメイン>.workers.dev`） |
| リポジトリ変数 | `API_BASE` | この API の URL。`https://liftplan-server-vjeuvyzwlq-as.a.run.app` |
| シークレット | `CLOUDFLARE_API_TOKEN` | Workers のデプロイ用 |
| シークレット | `CLOUDFLARE_ACCOUNT_ID` | 同上 |

```bash
gh variable set API_BASE --body 'https://liftplan-server-vjeuvyzwlq-as.a.run.app'
gh variable set ALLOWED_ORIGINS --body 'https://liftplan-web.<サブドメイン>.workers.dev'
gh secret set CLOUDFLARE_API_TOKEN
gh secret set CLOUDFLARE_ACCOUNT_ID
```

**`ALLOWED_ORIGINS` は鶏と卵になる。**画面をまだ一度も出していないと Workers の URL が
確定していない。Worker 名（`web/wrangler.jsonc` の `name`）とアカウントのサブドメインから
決まるので、先に手元で `pnpm deploy` を1回流して URL を確定させるのが早い。

## GitHub のリポジトリ名は GCP に握られている

**リポジトリをリネームすると、デプロイの認証が壊れる。**サービスアカウントの鍵を
GitHub に置かず Workload Identity 連携を使っているので、GCP 側が
「どのリポジトリからなら名乗ってよいか」を**完全一致の文字列で**持っている。

2箇所ある。

```bash
# 1. プロバイダの条件
gcloud iam workload-identity-pools providers describe liftplan-server \
  --project=liftplan-85309 --location=global --workload-identity-pool=github \
  --format='value(attributeCondition)'

# 2. サービスアカウントの紐付け
gcloud iam service-accounts get-iam-policy \
  github-deployer@liftplan-85309.iam.gserviceaccount.com --project=liftplan-85309
```

リネームするときは、**先に GCP を「両方許す」状態にしてから**リネームする。逆順にすると、
リネームから GCP 更新までのあいだデプロイできない。

```bash
# 先に両方を許す
gcloud iam workload-identity-pools providers update-oidc liftplan-server \
  --project=liftplan-85309 --location=global --workload-identity-pool=github \
  --attribute-condition="assertion.repository in ['dyoshyy/旧','dyoshyy/新'] && assertion.ref=='refs/heads/main'"

gcloud iam service-accounts add-iam-policy-binding \
  github-deployer@liftplan-85309.iam.gserviceaccount.com --project=liftplan-85309 \
  --role=roles/iam.workloadIdentityUser \
  --member="principalSet://iam.googleapis.com/projects/385680444543/locations/global/workloadIdentityPools/github/attribute.repository/dyoshyy/新"

# リネームして、デプロイが1本通るのを確認してから、古い名前を落とす
```

**古い名前を落とすのは、新しい名前でデプロイが1本通ってから。**先に落とすと、
失敗したときに戻す先が無くなる。

### 名前のうち、リポジトリ名と関係ないもの

リポジトリ名に合わせて変えたくなるが、**変えてはいけない・変えなくてよい**もの。

| 名前 | 判断 |
|---|---|
| Cloud Run のサービス `liftplan-server` | **変えない。**サービス名は変更できず作り直しになる。URL が変わり、`API_BASE`（画面のビルド）とヘルスチェックが全部つられる |
| Artifact Registry の `liftplan` / イメージのパス | リポジトリ名から導かれていない。そのまま |
| WIF プロバイダ名 `providers/liftplan-server` | ただのリソース名。一致している必要がない。変えるなら作り直し |
| Go のモジュールパス | **変えた**（D-123）。GitHub のリダイレクトで動き続けてはいたが、import に古い名前が残り続けるので単独の PR で置換した |

## 運用

- **コールドスタート**: `--min-instances=0` なので、しばらく使わないと初回が数秒かかる。ジムで最初に開くときだけ効く。気になるなら `--min-instances=1` にする（常時課金になる）
- **ログ**: `gcloud run services logs read liftplan-server --region asia-southeast1`
- **トークンの入れ替え**: `printf '%s' '<新しいトークン>' | gcloud secrets versions add liftplan-auth-token --data-file=-` してから再デプロイ。クライアント側も同時に変える必要があるので、切り替え中は 401 になる
- **ロールバック**: Cloud Run はリビジョンを保持するので、トラフィックを前のリビジョンに戻せる

  ```bash
  make releases                                    # 戻したいリビジョンを選ぶ
  gcloud run services update-traffic liftplan-server \
    --region=asia-southeast1 --project=liftplan-85309 \
    --to-revisions=<リビジョン名>=100
  ```

  **マイグレーションは戻らない**。スキーマは前に進んだままなので、
  戻す先のコードが新しい列を知らなくても動くことを確かめてから戻すこと。
  0002 のような型の変更は後方互換だが、列の削除を入れたら戻せなくなる

## ローカルで本番と同じイメージを動かす

```bash
make docker-run
```


## OAuth に切り替える（一度だけ）

`AUTH_TOKEN` は廃止した。**残っていると起動を拒む**（設定を外させるには止めるのが早い）。

### 1. 認可先を登録する

| | 作る場所 | コールバックURL |
|---|---|---|
| GitHub | Settings → Developer settings → OAuth Apps | `<API_ORIGIN>/auth/github/callback` |
| Google | Google Cloud → APIs & Services → 認証情報 → OAuth クライアントID（ウェブ） | `<API_ORIGIN>/auth/google/callback` |

**1文字でも違うと認可は通らない。**`API_ORIGIN` は Cloud Run が払い出した URL。

### 2. シークレットを置く

クライアントIDは秘密ではないのでリポジトリ変数、シークレットだけ Secret Manager に置く。

```bash
# --project を付けるのは、gcloud に既定のプロジェクトが入っていないと
# 「resource is not properly specified」で落ちるため。
# 毎回書くのが嫌なら gcloud config set project liftplan-85309。
printf '%s' '<GitHub のシークレット>' | \
  gcloud secrets create liftplan-github-client-secret \
    --project=liftplan-85309 --data-file=-
printf '%s' '<Google のシークレット>' | \
  gcloud secrets create liftplan-google-client-secret \
    --project=liftplan-85309 --data-file=-

# 変数名が GH_ なのは、GitHub が GITHUB_ で始まるリポジトリ変数を
# 作らせないため（予約接頭辞。作ろうとすると HTTP 422）。
# サーバーが読む環境変数は GITHUB_CLIENT_ID のままで、読み替えは
# deploy.yml の1箇所に閉じている。
gh variable set GH_CLIENT_ID --body '<...>'
gh variable set GOOGLE_CLIENT_ID --body '<...>'
gh variable set WEB_ORIGIN --body 'https://liftplan-web.<サブドメイン>.workers.dev'
```

**`echo` ではなく `printf` を使う。**`echo` は末尾に改行を足すので、シークレットの
最後に `\n` が付いたまま保存される。認可のときに「クライアントシークレットが違う」と
だけ言われ、値は合って見えるので原因に辿り着くのに時間がかかる。長さで確かめられる。

```bash
gcloud secrets versions access latest \
  --secret=liftplan-github-client-secret --project=liftplan-85309 | wc -c
```

GitHub のシークレットは40文字。`41` なら改行が混ざっている。

**`API_ORIGIN` は新しく作らない。**画面のビルドが使っている `API_BASE` と同じ URL なので、
`deploy.yml` がそちらから引く。同じ URL を指す変数が2つあると、片方だけ更新した日に
コールバックが黙って合わなくなる。

### 2.5 シークレットを読む権限を付ける

**作っただけでは Cloud Run から読めない。**付け忘れるとデプロイの最後で落ちる。

```
ERROR: (gcloud.run.deploy) Permission denied on secret:
  .../secrets/liftplan-github-client-secret/versions/latest
  for Revision service account 385680444543-compute@developer.gserviceaccount.com
```

```bash
for s in liftplan-github-client-secret liftplan-google-client-secret; do
  gcloud secrets add-iam-policy-binding "$s" \
    --project=liftplan-85309 \
    --member=serviceAccount:385680444543-compute@developer.gserviceaccount.com \
    --role=roles/secretmanager.secretAccessor
done
```

サービスアカウントは Cloud Run のリビジョンが使うもので、既定では
`<プロジェクト番号>-compute@developer.gserviceaccount.com`。相手が分からなく
なったら、既に動いている `DATABASE_URL` のシークレットを見れば分かる。

```bash
gcloud secrets get-iam-policy liftplan-database-url --project=liftplan-85309
```

**デプロイを走らせる前に確かめられる。**

```bash
for s in liftplan-database-url liftplan-github-client-secret liftplan-google-client-secret; do
  printf '%s: ' "$s"
  gcloud secrets get-iam-policy "$s" --project=liftplan-85309 \
    --format='value(bindings.members)' | tr ';' '\n' | grep -c compute@ || echo 0
done
```

3本とも `1` なら揃っている。

### 3. これまでの記録を自分のアカウントに結ぶ

**デプロイしたら、最初にログインする前にこれを流す。**

マルチユーザー化より前の記録は、マイグレーション `0007` が既定ユーザー
`8d5e743e-f1b0-4430-9998-89d313e89da8` に寄せてある。この行を入れておくと、
初回ログインがその利用者に結びつく。

```sql
INSERT INTO accounts (provider, subject, user_id)
VALUES ('github', '<自分の GitHub の数値ID>', '8d5e743e-f1b0-4430-9998-89d313e89da8');
```

数値IDは `curl -s https://api.github.com/users/<ユーザー名> | jq .id` で取れる
（`login` ではなく `id`。改名しても変わらないのはこちら）。Google なら
`provider` を `'google'`、`subject` を userinfo の `sub` にする。

**流す前にログインすると、空の利用者が新しく作られる。**これまでの記録は
消えないが、そのアカウントからは見えない。そうなったら `accounts` の
`user_id` を上の UUID に更新すれば戻る（作られたほうの行は消してよい）。

「最初にログインした人が既存の記録を引き継ぐ」にはしていない。デプロイ直後に
見知らぬ人が先にログインしただけで記録を持っていかれるため。

### 4. 順序

```
シークレットと変数を置く → サーバーをデプロイ → 上の INSERT を流す
  → 自分でログインして記録が見えることを確かめる → 画面をデプロイ
```

画面を先に出すと、ログインボタンの飛び先がまだ無い。
