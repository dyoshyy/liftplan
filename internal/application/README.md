# application 層

ユースケースを置く。いまは `usecase` パッケージ1つしか無い。

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

- **`query/`** — 過去の記録を集計して見せる経路（枠だけ用意してある）。
  `SessionPlanner` を通らず、リポジトリでもない。ユースケースとは責務が
  違うので別パッケージにする
- **port** — Clock や ID 生成のように、ドメインが持つべきでない外向きの口。
  いまは日付を外から渡す設計なので不要

認証（第6部）はここに置かない見込み。単一ユーザー向けの最小構成なら、
トークンを見るミドルウェアで済むので presentation 層に入る。ログインや
セッション管理まで育ったらここへ移す。

## ここに置かないもの

- **判断**。ユースケースはデータを集めてドメインに渡すだけ。判断が漏れ出したら、
  それはドメイン層に置くべきもの
- **リポジトリの interface**。永続化の口はドメイン層にある（依存性逆転）

ただし**入力の検証はここでやる**。判断ではないうえ、種目マスタとの突合のように
I/O を必要とするものはドメインに置けない（D-032 / D-047）。
