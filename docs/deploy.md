# デプロイ

Cloud Run（コンテナ）+ Neon（Postgres）。どちらもスケールゼロで、使わない間はほぼ課金されない。

## なぜこの構成か

- **Cloud Run**: Dockerfile をそのまま載せられる。TypeScript の層もフレームワークも要らない
- **Neon**: Postgres そのもの。開発中の Docker Postgres と接続文字列の形が同じで、検証したものがそのまま動く

Cloudflare Workers を選ばなかった理由は `docs/plans/06-auth-and-deploy.md` に書いた。要点は、Go の WASM ターゲットで `net` パッケージが使えず pgx が動かないこと。

## 1. Neon のプロジェクトを作る

コンソール（https://console.neon.tech）で作る。リージョンは `AWS ap-northeast-1 (Tokyo)`。

接続文字列は**プーラー無しのほう**を使う。

```
postgres://<user>:<password>@ep-xxxx.ap-northeast-1.aws.neon.tech/neondb?sslmode=require
```

`ep-xxxx-pooler...` のほうは pgbouncer のトランザクションモードで、prepared statement と相性が悪い。pgx は既定で statement cache を使うのでエラーになる。単一ユーザーの接続数なら pooler は要らない。

## 2. 認証トークンを作る

```bash
openssl rand -hex 32
```

32文字未満だとサーバーが起動しない。

## 3. 秘密を Secret Manager に入れる

環境変数に直書きしない。`gcloud run services describe` にもコンソールにも出てしまう。

```bash
PROJECT=<your-gcp-project>
gcloud config set project "$PROJECT"
gcloud services enable run.googleapis.com secretmanager.googleapis.com \
  artifactregistry.googleapis.com cloudbuild.googleapis.com

printf '%s' '<Neon の接続文字列>' | \
  gcloud secrets create liftplan-database-url --data-file=-
printf '%s' '<生成したトークン>' | \
  gcloud secrets create liftplan-auth-token --data-file=-
```

## 4. デプロイ

```bash
gcloud run deploy liftplan-server \
  --source . \
  --region asia-northeast1 \
  --min-instances=0 \
  --max-instances=2 \
  --cpu=1 --memory=512Mi \
  --set-secrets=DATABASE_URL=liftplan-database-url:latest,AUTH_TOKEN=liftplan-auth-token:latest \
  --allow-unauthenticated
```

`--allow-unauthenticated` は **Cloud Run 側の IAM 認証を切る**という意味で、アプリの認証は別に効いている。ここを閉じると Google のアカウントが要るようになり、Android から叩けない。

`--max-instances=2` にしているのは、単一ユーザーで台数が増える理由が無いのと、Neon の接続数を使い切らないため。1インスタンスあたり最大8接続を張る。

## 5. 確認

```bash
URL=$(gcloud run services describe liftplan-server \
  --region asia-northeast1 --format='value(status.url)')

curl -s "$URL/healthz"                      # {"status":"ok"}
curl -s -o /dev/null -w '%{http_code}\n' "$URL/api/program"   # 401
curl -s -H "Authorization: Bearer <トークン>" "$URL/api/program"
```

マイグレーションは起動時に自動で流れる。空のデータベースなら初期プログラムも入る。

## 運用

- **コールドスタート**: `--min-instances=0` なので、しばらく使わないと初回が数秒かかる。ジムで最初に開くときだけ効く。気になるなら `--min-instances=1` にする（常時課金になる）
- **ログ**: `gcloud run services logs read liftplan-server --region asia-northeast1`
- **トークンの入れ替え**: `printf '%s' '<新しいトークン>' | gcloud secrets versions add liftplan-auth-token --data-file=-` してから再デプロイ。クライアント側も同時に変える必要があるので、切り替え中は 401 になる
- **ロールバック**: Cloud Run はリビジョンを保持するので、コンソールからトラフィックを前のリビジョンに戻せる

## ローカルで本番と同じイメージを動かす

```bash
make docker-run
```
