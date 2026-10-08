---
name: dev-simulation
description: liftplan で計画の導出（internal/domain/training/planning）・シード・週目標・devsim を変えたあと、処方される重量や週ボリュームが何週にもわたって意図どおりかを確かめるとき。「重量が伸びすぎる／伸びない」「補助の割り振りが偏る」「分割や頻度を変えたらどうなるか」を数字で見たいとき。/dev.html や /api/dev/simulate の結果を読むとき。
---

# 開発用シミュレーションで確かめる

本番と同じ `planning.SessionPlanner` を、模擬ユーザー（実力どおりに記録する本人）で何週も回す。
`seed` の `TestSimulation` が数字の回帰を止める番人なのに対して、こちらは**何が起きているかを読む**道具。

**速いのは API を直接叩く経路。**ブラウザも Vite も要らない。

## 起動と停止

```bash
S=<スクラッチパッド>; go build -o $S/api ./cmd/api
DEV_SESSION_TOKEN=dev-token-0123456789abcdef0123456789ab \
  API_ORIGIN=http://localhost:18080 WEB_ORIGIN=http://localhost:5173 \
  ALLOWED_ORIGINS=http://localhost:5173 PORT=18080 \
  GITHUB_CLIENT_ID=dev GITHUB_CLIENT_SECRET=dev GOOGLE_CLIENT_ID=dev GOOGLE_CLIENT_SECRET=dev \
  $S/api &   # 実行中の Bash は run_in_background で
curl -s -H "Authorization: Bearer dev-token-0123456789abcdef0123456789ab" \
  'http://localhost:18080/api/dev/simulate?declared=bench&growth=0' | jq .settings
```

- DATABASE_URL を付けない（インメモリ。付けると DEV_SESSION_TOKEN が無視され全部 401）
- 止めるのは PID で：`kill $(ss -ltnp | grep ':18080 ' | sed -n 's/.*pid=\([0-9]*\).*/\1/p')`。`pkill -f` は自分のシェルも殺す
- 変更の前後を比べるなら、変更前に1つビルドしておき、別ポートで並べて同じクエリを投げる

## クエリ

| キー | 例 | 既定（`/api/dev/options` が返す） |
|---|---|---|
| `declared` | `bench,squat,deadlift` | 必須 |
| `focus` / `split` | `bench` / `upper_lower`（空＝なし） | なし |
| `frequency` / `weeks` | `4` / `12`（最大12） | 4 / 4 |
| `days` | `1,3,5`（開始日からの日数。既定の開始日は月曜なので 0=月・1=火…。頻度はこの数） | 頻度ごとの既定 |
| `exercises` / `sets` | `5` / `4` | 4 / 3 |
| `start` | `2026-09-07` | 2026-08-03（月） |
| `growth` | 実力の伸び %/週 | **0.5** |
| `first_pct` | 履歴の無い初回に本人が選ぶ重さ（実力の%） | 70 |
| `body_weight` | kg | 75 |
| `orm` | `bench:120,pull_up:0`（自重種目は加重ぶん） | 種目ごと（options の `default_1rm_kg`） |
| `custom` | `名前\|区分:寄与,区分:寄与\|刻み` を `;` で並べる（例 `アイソラテラル・ロー\|TRAP_MID:1,LAT:0.5,BICEPS:0.5,REAR_DELT:0.5\|2.5`）。ID は並び順で `u-sim01`… が振られ、応答の `settings.custom` に `{id, name, stimulus, increment_kg}` で出る。`orm` の上書きもこの ID で指す | 無し |
| `edit` | プリセットの上書き。`ID\|区分:寄与,区分:寄与\|刻み` を `;` で並べる（例 `seated_row\|TRAP_MID:1,LAT:1,BICEPS:0.5\|2.5`）。名前・自重係数は変えない。応答の `settings.edits` に出る | 無し |
| `unused` | 使わない種目の ID を `,` 区切り（本番で使う種目から外すのと同じ。宣言した種目は外せない）。応答の `settings.unused` に出る | 全部使う |

**既定は中立ではない。**伸び率の既定は 0.5%/週。「実力が一定なら」を問うなら `growth=0` を必ず書く。
**答える前に `.settings` を読む。**既定値を解決したあとの全設定（全種目の1RMを含む）が返る。問いと違っていたら、クエリが違う。
形の壊れた値・知らないキー（`growt=0` のような書き間違い）・同じキーの2回指定は 400 で、本文がどのキーが悪いかを名指しする。

## 応答の読み方

- `days[].main`（軸）/ `variation` / `accessories` の要素は**1種目に1つ**（セット数は `sets`。全セット同じ重さ）：
  - `weight_kg`：処方（加重）。`null` は履歴が無く本人が決める回
  - `performed`：模擬ユーザーの記録 `{weight_kg, reps, rir}`。`null` の回も本人が選んだ重さで埋まる
  - `athlete_1rm_kg`：その日の**実力**（加重の1RM）。「実力に対して何%か」はこれで割る
  - `pct_of_1rm`：処方 ÷ **推定**1RM（体重込み）。役割の強度（軸 0.81〜0.88・派生 0.80・補助 0.71）に並ぶかを見る。実力比ではない
- `weeks[].regions[]`：`{region, target, done}`。週 `index` は1始まりで、開始日から7日ずつ
  - **許容帯は達成率 60% 以上 145% 以下**（`done/target`、端を含む）。**target 0 の区分は数えない**。応答には帯は無い

```bash
jq '[.days[] | .date as $d | .main[] | select(.exercise_id=="bench")
     | {d:$d, kg:.weight_kg, did:.performed, real:.athlete_1rm_kg}]'
jq '.weeks[] | select(.index==3) | .regions[] | select(.target>0)
     | {region, r:(.done/.target)} | select(.r<0.6 or .r>1.45)'
```

## 画面で読む（人に見せるとき）

`/dev.html?` に同じクエリを付けると、その設定で即実行する。ログイン済みのブラウザで開き、ページの文字（get_page_text）の先頭にある「要約」が、前提・宣言種目ごとの推移・週ごとの許容外の区分を文字で持つ。ローカルでは `cd web && VITE_API_BASE=http://localhost:<APIのポート> pnpm dev`（`.env` は拾われず、無いと画面が真っ白）。API の `WEB_ORIGIN`/`ALLOWED_ORIGINS` は画面のオリジンに合わせる。`http://localhost:5173/#token=<DEV_SESSION_TOKEN>` を一度開いてから `/dev.html?...` を開く。

## よくある取り違え

| 取り違え | 正しくは |
|---|---|
| growth を省いて「伸びない場合」を答える | 既定は 0.5%/週。`growth=0` |
| `pct_of_1rm` を実力比として報告 | 実力比は `weight ÷ athlete_1rm_kg` |
| 処方 `null` の回を0kgとして数える | 本人が選ぶ回。重さは `performed` |
| 目標0の区分を「0%で許容外」 | 数えない |
| 模擬ユーザーの仮定の結果をアプリの性質と言う | 模擬ユーザーは実力どおり・目標RIRちょうどで、処方を全部こなす。やめどきの判断は入らない |
