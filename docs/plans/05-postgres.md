# 第5部: Postgres への差し替え

> **エージェント向け:** `superpowers:subagent-driven-development` で1タスク1PRとして実装する。各PRの後に敵対的検証を行い、穴を潰してから次へ進む。

**ゴール:** リポジトリ実装を Postgres に差し替え、再起動しても記録が残る状態にする。

**アーキテクチャ:** 変更は `internal/infrastructure/postgres/` の新設と `cmd/api/main.go` の配線だけに閉じる。ドメイン・アプリケーション・プレゼンテーションの各層は1行も変えない。**変えなくて済むことが、第1〜4部で守ってきた依存方向の答え合わせになる。**

**技術スタック:** Go 1.26 / pgx v5 / 埋め込み SQL によるマイグレーション / テストは Docker の Postgres 17

## この部の前提

第4部までで、ドメインが正しいことは DB 抜きで証明されている。ここで新たに入るバグは SQL の側にしかない。だから**インメモリ実装と Postgres 実装に同じ契約テストを流す**のが中心的な検証方法になる。

## 全体の制約

- ドメイン層は引き続き標準ライブラリのみ。pgx は infrastructure 層だけが知る
- `internal/architecture_test.go` の許可リスト（`layers`）に新しいパッケージを追加すること。追加しないと `TestOnion_EveryPackageDeclaresItsLayer` が落ちる（D-051）
- 保存の契約（D-033）は Postgres 実装でも同じ。冪等・`ErrConflictingSetLog`・全か無か・取得順の安定
- 接続文字列は `DATABASE_URL` 環境変数。Neon もローカルの Docker も同じ形

---

## Task 25: 依存とマイグレーション基盤

**Files:**
- Create: `internal/infrastructure/postgres/migrate.go`
- Create: `internal/infrastructure/postgres/migrations/0001_init.sql`
- Create: `internal/infrastructure/postgres/pool.go`
- Test: `internal/infrastructure/postgres/migrate_test.go`
- Modify: `go.mod` / `internal/architecture_test.go`

**Produces:**
- `func Open(ctx context.Context, url string) (*pgxpool.Pool, error)`
- `func Migrate(ctx context.Context, pool *pgxpool.Pool) error`

**判断:**

- **ドライバは pgx v5 を直接使う**（`database/sql` を挟まない）。差し替え点はリポジトリインターフェースであって SQL ドライバではないので、`database/sql` の抽象は何も守ってくれない。挟むと prepared statement のキャッシュと型の扱いを捨てることになる
- **マイグレーションは埋め込み SQL + 自前のランナー**。golang-migrate は CLI とドライバ登録を持ち込むが、必要なのは「連番の SQL をトランザクションで順に流し、適用済みを記録する」だけ。80行で書けるものに外部依存を足さない
- **アドバイザリロックを取る**。複数インスタンスが同時に起動しても二重適用しない

**スキーマ:**

```sql
CREATE TABLE set_logs (
    id            text PRIMARY KEY,
    performed_on  date        NOT NULL,
    exercise_id   text        NOT NULL,
    weight_kg     numeric(6,2) NOT NULL,
    reps          integer     NOT NULL,
    rir           integer     NOT NULL
);
CREATE INDEX set_logs_performed_on_idx ON set_logs (performed_on);
CREATE INDEX set_logs_exercise_idx ON set_logs (exercise_id, performed_on);

CREATE TABLE daily_conditions (
    date           date PRIMARY KEY,
    body_weight_kg numeric(5,2),
    sleep_hours    numeric(4,2)
);

-- プログラムはユーザーごとに1つ。単一ユーザー前提なので1行に固定する。
CREATE TABLE program (
    id            boolean PRIMARY KEY DEFAULT true CHECK (id),
    per_week      integer NOT NULL,
    weekly_target jsonb   NOT NULL,
    selected      jsonb   NOT NULL
);
```

`weight_kg` を `numeric` にするのは、`double precision` だと 87.5kg のような値が往復で揺れうるため。ドメインは 1e-6 で量子化しているので、`numeric(6,2)` で足りる。

種目マスタはテーブルにしない。シードは Go のコードで、バイナリに同梱される静的なマスタとして扱う。DB に置くとマイグレーションのたびに種目の追加・改名が絡み、`ErrExerciseNotFound` の意味が「まだ流していない」と混ざる。

**検証:**
- 空の DB に流して全テーブルができること
- 二度流しても壊れないこと（冪等）
- 途中の SQL が失敗したら、そのファイルの変更が丸ごと巻き戻ること

---

## Task 26: 共有の契約テストスイート

**Files:**
- Create: `internal/infrastructure/repositorytest/contract.go`
- Modify: `internal/infrastructure/memory/repositories_test.go`

**Produces:**
- `func RunSetLogContract(t *testing.T, newRepo func(*testing.T) training.SetLogRepository)`
- `func RunConditionContract(t *testing.T, newRepo func(*testing.T) training.ConditionRepository)`
- `func RunProgramContract(t *testing.T, newRepo func(*testing.T) training.ProgramRepository)`

**なぜ必要か:** D-033 で定めた契約は、いまインメモリ実装のテストにしか書かれていない。Postgres 実装が同じ契約を守っているかを、**同じテストコードで**確かめる。実装ごとにテストを書き直すと、片方だけが契約を満たす状態に気づけない。

契約に入れるもの（D-033 / D-040 / D-041 より）:
- 同じ ID・同じ内容の再送を受け入れる
- 同じ ID・違う内容は `ErrConflictingSetLog`
- 同一呼び出し内の衝突も検出する
- 全か無か（衝突時に1件も書かれない）
- `nil` 要素を拒否する
- 取得順が呼び出しごとに変わらない
- 同じ日付のコンディションを項目ごとに合成する（体重だけ送っても睡眠が消えない）
- 日付の無いコンディションを拒否する
- `Get` は未設定で `ErrProgramNotConfigured`（`(nil, nil)` を返さない）
- 並行に呼んでも壊れない

---

## Task 27: SetLogRepository の Postgres 実装

**Files:**
- Create: `internal/infrastructure/postgres/set_log_repository.go`
- Test: `internal/infrastructure/postgres/set_log_repository_test.go`

**衝突検出の方法:** トランザクション内で対象 ID を `SELECT ... FOR UPDATE` し、既存行と内容を比較してから `INSERT`。`ON CONFLICT DO NOTHING` だと「入らなかった理由が同一だからか衝突だからか」を後から判別する必要があり、往復が増える。

トランザクションで包むことが、そのまま「全か無か」の実装になる。

---

## Task 28: ConditionRepository / ProgramRepository の Postgres 実装

**Files:**
- Create: `internal/infrastructure/postgres/condition_repository.go`
- Create: `internal/infrastructure/postgres/program_repository.go`
- Test: 同名の `_test.go`

**コンディションの合成:** `INSERT ... ON CONFLICT (date) DO UPDATE SET body_weight_kg = COALESCE(EXCLUDED.body_weight_kg, daily_conditions.body_weight_kg)` の形。`EXCLUDED` が NULL のときに既存値を残すのが「項目ごとの上書き」（D-040）。

**プログラム:** 週目標と選択種目は jsonb。列に開くと筋区分の追加でマイグレーションが要る。読み出し時に `NewWeeklyVolumeTarget` / `NewProgram` を通すので、DB に不正な値が入っていても境界で弾かれる。

---

## Task 29: 配線と運用

**Files:**
- Modify: `cmd/api/main.go`
- Create: `compose.yaml`
- Modify: `README.md`

`DATABASE_URL` があれば Postgres、無ければインメモリ。インメモリを残すのは、ドメインの検証を DB 無しで回せる状態を捨てないため。

**受け入れ基準:** この差し替えで変わるのが `cmd/api/main.go` と新設の `postgres` パッケージだけであること。他の層に1行でも変更が要るなら、そこが依存方向の破れ。
