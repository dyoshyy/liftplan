# application 層

ユースケースを置く。パッケージは4つ。

- **`usecase/`** — 書き込みと、その日のセッションの導出。ログインの受け入れ
  （`SignIn`）もここ
- **`query/`** — 過去の記録を集計して見せる経路（履歴・統計・種目マスタ）。
  `SessionPlanner` を通らず、リポジトリでもない。ユースケースとは責務が
  違うので別パッケージにしてある
- **`apperror/`** — ドメインのセンチネルを、presentation が見る1つの型へ
  翻訳する
- **`devsim/`** — 開発用のシミュレーション。本番の経路には入らない

## なぜ1段深いのか

`internal/usecase` にフラットにしても動く。Go の実践ではむしろそちらが多い。
それでも `internal/application/usecase` にしているのは、**import 行そのものが
層を語るから**。

```go
import "github.com/dyoshyy/liftplan/internal/presentation/httpapi"
import "github.com/dyoshyy/liftplan/internal/infrastructure/postgres"
```

この2行が同じファイルに並んでいたら、依存方向の違反だと**読んだ瞬間に分かる**。
フラットだと `internal/httpapi` が `internal/postgres` を import しているだけに
見えて、どちらが外側かを知っていないと気づけない。

`internal/architecture_test.go` は落としてくれるが、レビューで気づけることは
それとは別の価値がある。

## ここに増えうるもの

- **port** — Clock や ID 生成のように、ドメインが持つべきでない外向きの口。
  いまは日付も時刻も外から渡す設計なので不要

認証は2つの層に分かれている。ログインを受け入れて利用者とセッションを作る
手順（`usecase.SignIn`）はここ。リクエストごとにセッショントークンから
「誰か」を決めるのは presentation 層のミドルウェア（`httpapi/auth.go`）で、
プロバイダとの通信はインフラ層にある（D-136）。

## ここに置かないもの

- **判断**。ユースケースはデータを集めてドメインに渡すだけ。判断が漏れ出したら、
  それはドメイン層に置くべきもの
- **リポジトリの interface**。永続化の口はドメイン層にある（依存性逆転）

ただし**入力の検証はここでやる**。判断ではないうえ、種目マスタとの突合のように
I/O を必要とするものはドメインに置けない（D-032 / D-047）。
