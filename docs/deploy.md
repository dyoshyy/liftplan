# デプロイ

Cloud Run（コンテナ）+ Neon（Postgres）。どちらもスケールゼロで、使わない間はほぼ課金されない。

## なぜこの構成か

- **Cloud Run**: Dockerfile をそのまま載せられる。TypeScript の層もフレームワークも要らない
- **リージョンはシンガポール**（`asia-southeast1`）。Neon に東京が無いので、DB と同居させる。1リクエストで DB を4本叩くため、ユーザーに近づけるより DB に近づけるほうが速い（東京 CR + シンガポール DB は約290ms、シンガポール同居は約74ms）
- **Neon**: Postgres そのもの。開発中の Docker Postgres と接続文字列の形が同じで、検証したものがそのまま動く

Cloudflare Workers を選ばなかった理由は `docs/plans/06-auth-and-deploy.md` に書いた。要点は、Go の WASM ターゲットで `net` パッケージが使えず pgx が動かないこと。

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
  --set-secrets=DATABASE_URL=liftplan-database-url:latest,AUTH_TOKEN=liftplan-auth-token:latest \
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
| Go のモジュールパス | GitHub がリダイレクトするので動き続ける。変えるなら 57 ファイルの機械的な置換で、単独の PR にする |

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
