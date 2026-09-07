# 第8部: クライアントをミニマルに絞る

**日付:** 2026-09-07
**状態:** 計画（レビュー前）
**関係:** PR #48（React への作り直し）のスコープを縮める

## なぜ縮めるか

PR #48 は `app.js` 801行の**全機能**を React に移した。2,043行、28ファイル。
動くことは実機で確認したが、**まだ一度もジムで使われていない。**

`docs/plans/07-client.md` に書いたとおり、クライアントを作る目的は
「スロットの強度も週目標もシミュレーションでしか検証されていない」状態を
終わらせることにある。**定数を直す唯一の方法は使うこと。**

全機能を移したことで、使い始めるまでにレビューすべき面積が最大になった。
CLAUDE.md の「必要になるまで作らない」に照らすと、順序が逆になっている。

**縮める判断の基準：ジムで1回のセッションを記録し終えるのに要るか。**
要らないものは、使い始めてから戻す。

## 残すもの

| 画面・機構 | なぜ要るか |
|---|---|
| トークン入力 | これが無いと何も叩けない |
| 今日のメニュー表示 | これが目的 |
| セットの記録・修正・取り消し | これが目的 |
| **送信の待ち行列（IndexedDB）** | ジムの電波は途切れる。**記録が消えないことがこのアプリの土台** |
| 状態バー（未送信件数・オフライン表示） | 送れていないことに気づけないのが一番まずい |
| 送れなかった記録の表示 | 黙って消えるより悪いものは無い |
| Service Worker と manifest | PWA として入れられること自体が要件 |

## 落とすもの

| 落とすもの | 行数 | 落としてよい理由 | 戻す条件 |
|---|---|---|---|
| 履歴タブ（`History` / `Sparkline`） | 185 | 記録を取ることとは独立。**サーバーの `/api/stats` は残るので、データは溜まり続ける**。見るのは後からでよい | 数週間ぶん溜まって、推移を見たくなったとき |
| 設定タブ（`Settings`） | 113 | 初期プログラム（週3・全種目）がサーバー側で入る。**変えたくなるまで要らない** | 種目を外したくなったとき |
| コンディション入力の**睡眠だけ**（`Condition`） | 約27 | `sleep_hours` はサーバーのどの判断にも使われていない。**体重は残す**（上記 P1） | 睡眠を見る判断を入れるとき |
| デロードの提案UI | 約45 | 同上。停滞は最短でも数週間後にしか出ない | 停滞が実際に出たとき |
| 区分の日本語（`regions.ts`） | 27 | 履歴タブでしか使っていない | 履歴タブと同時 |
| 更新の通知（`UpdatePrompt` の**UIだけ**） | 27 | 利用者が1人で、更新は自分が出したときにしか来ない | 他人が使い始めたとき |
| タブ（`Tabs`） | 28 | 画面が1つになるので切り替え先が無い | 2つ目の画面を戻すとき |

**落とさないもの。**`outbox/`（214行）と `domain/`（62行）は触らない。
ここが記録を失わないための本体で、行数を減らす目的で薄くすると本末転倒になる。

## 作業後の見込み

2,043行 → **約1,150行**（テスト186行を含む）。ファイル28 → 17。

## レビューで直した点（fable、2026-09-07）

**P0: `UpdatePrompt` を消すと Service Worker が登録されなくなる。**
`useRegisterSW()` がこのアプリで唯一の登録箇所で、`vite.config.ts` は
`injectRegister: null`、`main.tsx` にも登録が無い。消すとプリキャッシュされず、
インストール可能条件も満たさない。「PWA として入れられること」と正面衝突する。

**実測で確認した。**本番ビルドをヘッドレス Chromium で開き、
`navigator.serviceWorker.getRegistrations()` を数えた。

| 画面 | 登録数 |
|---|---|
| 設定（トークン入力） | **0** |
| ログイン後 | 1 |

**これは PR #48 の既存バグでもある。**`UpdatePrompt` は `hasToken` が真のときしか
描画されないので、**初回訪問では SW が登録されない**。ミニマル化と関係なく直す。

→ **`main.tsx` で `registerSW()` を呼ぶ**。UI だけ落とし、登録は起動時に必ず走らせる。

`registerType: 'prompt'` は残す。`skipWaiting` を呼ばないので、待機中の新版は
**全てのクライアントが閉じたとき**に有効になる。ホーム画面の PWA が生きている間は
旧版のまま。記録の途中で画面が差し替わらないほうを取る。

**P1: 体重は自重種目の処方と推定にも使われている。**デロードだけではない。
`effective_load.go` の `EffectiveLoad` / `AddedWeight` が、`bodyweightFactor != 0` の
種目（`dip` 0.93 / `pull_up` 0.95 / `back_extension` 0.55、いずれも初期プログラムに入る）
で体重を足して推定し、引いて処方する。

落とし穴がある。体重が一件も無い間は既定 70kg で動くが、**数週間 70kg で記録したあとに
体重を1回でも入れると**、`HasBodyWeight()==true` かつ `BodyWeightAsOf(過去日)==false` に
なり、それ以前の自重種目のセットが推定から除外される。SetLog は残るので「記録が消える」
ではないが、**その種目の推移は事実上失われ、推定1RMが後退する**。

→ **体重の入力だけ残す。睡眠は落とす。**`sleep_hours` は `deload_policy.go` も
`session_planner.go` も参照していない。

**P2: `describe()` の `conditions` 分岐は消さない。**`adoptLegacyQueue` が旧版の
`liftplan.queue` から `/api/conditions` の項目を引き取りうる。

**P2: `Settings` を落とすと手動でトークンを消す手段が無くなる。**残るのは 401 →
`clearToken` → 設定画面の経路だけ。記録は失われないので許容する。

## 手順

1. **落とすファイルを消す**
   `features/history/`、`features/settings/`、`app/Tabs.tsx`、
   `app/UpdatePrompt.tsx`、`domain/regions.ts`
   （`features/today/Condition.tsx` は**残して体重だけにする**）

1b. **`main.tsx` で `registerSW()` を呼ぶ**（P0。UI を消しても登録は残す）
2. **`useLiftplan.ts` を絞る**
   `stats` / `days` / `program` / `exercises` を落とし、`/api/exercises`（種目名）と
   `/api/set-logs`（今日の実績と前回）だけを読む。
   **`/api/stats` と `/api/program` の呼び出しを消す**
3. **デロードを消す。参照は4箇所ある**（片側だけ直すと型エラーになる）
   - `Today.tsx` の `Deload` コンポーネントと `onReloadToday` prop
   - `App.tsx` の `onReloadToday={...}`
   - `useLiftplan.ts` の `loadToday` の引数と `acceptedDeload` の import
   - `storage/local.ts` の `acceptedDeload` / `setAcceptedDeload`
4. **`App.tsx` をタブ無しにする**
5. **`api/types.ts` を整理する**

   | 消す | 残す |
   |---|---|
   | `StatsResponse` `Trend` `TrendPoint` `Volume` | `Day` `ExerciseLog` `HistoryResponse`（`doneToday` を組むのに要る） |
   | `Program` | `LastPerformance` |
   | `SetLogInput`（今も未使用） | `DeloadProposal` / `Session.deload_proposal`（UIが読まなくてもサーバーの契約） |
   | | `ConditionInput`（体重を残すので要る） |
6. **テストを走らせ、実機で描画して確認する**
7. **古い記述を直す**
   `web/wrangler.jsonc` の「画面は3つのタブ」、`web/README.md` の分割表、
   仕様書 `2026-09-07-pwa-client-design.md` の「画面が3つ」
8. **`docs/decisions.md` に D-120 として残す**（仕様書を上書きすることに触れる）

## 検収

```
cd web && pnpm typecheck && pnpm test && pnpm build
gofmt -l . && go vet ./... && go test ./...
```

加えて**ヘッドレス Chromium で実機確認**する。

- トークンを入れて今日の画面が出る
- 1セット記録 → サーバーに届く
- 同じセットを修正 → **重複せず1件のまま更新される**
- オフラインにして記録 → 未送信件数が増える → 復帰して送られる
- コンソールエラーが無い

## 触らないもの

- **サーバー側は一切変えない。**`/api/stats` も `/api/program` も残す。
  クライアントが叩かなくなるだけ。消すと、戻すときにサーバーから作り直しになる
- **保存されているデータの形。**DB名・ストア名・localStorage のキー
- **CORS と配備（PR #48 の第2・第3コミット）**

## 割れうる点

**「履歴を落とすと、記録が本当に入ったか確認する手段が減る」。**
今日の画面に記録済みのセットが緑で出るので、その日のぶんは確認できる。
過去のぶんはサーバーに `curl` すれば読める。使い始めの段階では足りると判断した。
