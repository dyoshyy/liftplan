---
date: 2026-10-02
status: 設計（未実装）
---

# 週目標を比で持つ（プリセットに依存しない週目標）

**日付**：2026-10-02

週目標を「区分ごとの目標セット数（絶対量）」ではなく「区分どうしの配分の比」で持つ。
補助の割り振りは、その週に積んだ刺激を割合に直して配分と比べる。
1セットあたりの平均寄与 k（`averageStimulusPerSet`）は要らなくなる。

最終目的は、プリセットを大量に足しても（Hammer Strength のマシンなど）
週目標と `TestSimulation` の数字が動かないこと。

## 何が問題だったか

### 1. プリセットを足すと、全員の週目標と計画が動く

いまの週目標（`seed/weekly_target.go`）は

```
T_r = 頻度 × 1回の種目数 × 1種目あたりのセット数 × k × (w_r / Σw)
k   = プリセット全38種目の「1セットあたり寄与の合計」の平均 = 1.755263
```

k は `seed.Exercises()` を実行時に数える。利用者の種目の行（コピー方式。
`2026-09-26-custom-exercises-design.md`）ではなく同梱のカタログなので、
**プリセットを1つ足した瞬間、既存の利用者全員の週目標が動く。**既存の利用者には
新しいプリセットの行が入らないので、使えない種目のせいで目標だけが動く。
「使う種目」の既定を最小限にする方針（別PRで進行中）とも合わない。使わない種目に
目標が依存する。

動く量は小さくない。付録の案（アイソラテラル9種目、寄与の合計18）を足すと
k は 1.755→1.802（+2.7%）になる。試作で、種目のプールは変えずに週目標の倍率だけを変えて測った。

- **同じ履歴から今日を1回計画したとき**、1.027 倍で今日の補助が変わった回は
  全身法・週3回・8週の24回中 **9回**、全構成（全身法と3分割、週1〜7回）では961回中 **338回**。
  1.1 倍で 19回、0.9 倍で 21回（24回中）
- **そのまま数週回すと**、1.027 倍では7回目で計画が分かれ、9回目から先はほぼ全部が別物になる
  （分かれたあとは履歴が違うので、連鎖を含む）。±10% なら2回目で分かれる

### 2. k は実際に選ばれる種目とも合っていない

k はカタログの平均で、実際に選ばれる種目（BIG3 と多関節）の平均寄与より小さい。
処方どおりこなした通し検証では、全身法で「全区分の実測の合計 ÷ 週目標の合計」が
**1.05〜1.10**（週1〜7回）になる。画面の「充足」の合計の棒（`WeeklySummary`）は、
全部こなすと 100% で止まったまま動かない。

### 3. 「この表が効くのはゲートと同点の順序付けまで」は正しくない

`DefaultWeeklyTarget` のコメントはそう書いているが、`AccessoryAllocator.Allocate` を
読むと週目標は3か所で効いている。

| どこ | 何が決まるか |
|---|---|
| `deltaLoss`／`regionLoss`（C と 4T を比べる） | どの組が最良か、β の帯に入る組（候補の絞り込み） |
| 手順2（最良の ΔL* ≥ 0 なら止める） | **何本入れるか（量）** |
| `current` の初期化・確定時の更新 | 目標のある区分だけを追う（ゲート） |

同点処理（手順4 a〜d）は最終実施日・空き枠・日付・種目IDだけを見ていて、週目標は読まない。

量にも効いている。打ち切りで今日（回0）に枠を残して止まった Plan の呼び出しは、
`TestSimulation_Report`（全身法、224回）で28回、`TestSimulation_SplitReport` で403回あった。
five_way の肩の日は 3〜9セットで終わる日がある。k が動けば、この量も動く。

## 現状の整理：週目標を読む全箇所

| 箇所 | 使い方 | 依存しているもの |
|---|---|---|
| `seed/weekly_target.go` `DefaultWeeklyTarget`・`distribute`・`averageStimulusPerSet` | 頻度×種目数×セット数×k を `regionShare` で割る | **絶対量を作る**（k・頻度・1回の量） |
| `program/program.go` `WeeklyVolumeTarget` | 入れ物。正で有限かを検証。`Sets`・`Regions`・`IsEmpty` | 型は量を仮定しない（doc コメントは「目標セット数」） |
| `planning/session_planner.go` `Forecast` | `IsEmpty` の検査と `Allocate` への受け渡し | どちらでもない |
| `planning/horizon_projector.go` | 読まない（枠と軸・バリエーションの刺激だけ） | — |
| `planning/accessory_allocator.go` `current` の初期化・確定時の更新 | 目標のある区分（T>0）だけ追う | 比で足りる（区分の集合だけ） |
| 同 `deltaLoss`・`regionLoss` | C_r と 4T_r を比べる → 最良・β の帯・打ち切り | **絶対量** |
| 同 手順4（同点処理） | 最終実施日・空き枠・日付・ID | 読まない |
| `usecase/get_session.go`・`get_forecast.go` | `DefaultWeeklyTarget(f, v)` を組んで渡す | 受け渡し |
| `query/stats.go` `WeeklyVolume` の `TargetSet` | T_r をそのまま返す | **絶対量**（表示） |
| 同 並び替え（埋まっていない順） | done_r / T_r の昇順 | 比で足りる（T∝p なので q/p の昇順と同じ並び） |
| `httpapi` `target_sets`・`done_sets` → `web/src/features/history/History.tsx` `WeeklyVolume` | 区分ごとに「done / target」と棒（`volume.ts` `fillPercent`） | 数字は**絶対量**、棒の割合は比で足りる |
| `History.tsx` `WeeklySummary`・`weekly.ts` `weeklyTotal` | Σdone / Σtarget と % | **絶対量** |
| `devsim/simulator.go` `RegionVolume.Target` → `/dev.html`（`simulate.ts` の `rate`・`tone`・`outOfRange`、`chart.ts`、`RegionHeatmap.tsx`、`summary.ts`） | 週ごとの done/target と帯 60〜145% | **絶対量** |
| `seed/simulation_test.go` `rate`・`outOfBand` | 達成率の帯 60〜145%、補助1本ぶんの段 | **絶対量** |
| `seed/seed_test.go` `TestDefaultWeeklyTarget_CoversEveryRegion` | 全設定で全区分が正 | 絶対量（頻度・1回の量ごと） |
| `seed/weekly_target_internal_test.go` `TestDefaultWeeklyTarget_ShareDoesNotMoveTotal` | 配分を動かしても総量が動かない | 絶対量（総量） |
| `seed/seed_test.go` `TestDefaultWeeklyTarget_DistributionIsUnchanged` | 区分どうしの比 | 比 |
| `seed/seed_test.go` `TestSeed_EveryTargetRegionHasANonMainExercise` | T>0 の区分にメイン以外の手段がある | 比で足りる（集合） |
| `planning/*_test.go`（`mustTarget` に数値を直書き） | 「4週で 4T」を前提に実績を組む | **絶対量** |
| `query/stats_test.go` | `TargetSet == DefaultWeeklyTarget(...).Sets(r)` | 絶対量 |

## 決めたこと

| 論点 | 決定 | 理由 |
|---|---|---|
| 目標の持ち方 | `regionShare` の比 p_r だけを持つ。k・頻度・1回の量は目標に入れない | プリセット・使う種目・設定のどれにも依存しない。「比だけが意味を持つ」は配分表のコメントが既に言っている |
| 供給の数え方 | いまの C_r（窓の実績＋1週ぶんの軸・バリエーション＋割り振った補助、寄与×セット数）をそのまま使い、目標のある区分の合計 S で割る | 数え方を変えると画面とエンジンの数字がずれる（`CoverageBetween` のコメント） |
| 損失の形 | いまの形（指数3・超過を α で軽く・重み付き）を保ち、4T_r を p_r·S に置き換える | 指数3と重みは割り振りの設計書の PR 3 で実測から決めた。理由は比にしても変わらない |
| 枠の埋め方 | **候補が尽きるまで埋める**（損失が減らなくなっても止めない）★本人確認 | 比には「足りた」が無い。打ち切りを残すと量が崩れる（試作で 1回 4.7セットまで落ちた）。量は 1回の種目数（本人の設定）で決まる |
| 最良の ΔL* ≥ 0 のときの「ほぼ同じ」 | ΔL ≤ ΔL*/β を残す | 埋め切るので正の ΔL でも選ぶ。β×ΔL* だと帯が空になる |
| S = 0 | 全区分 q_r = 0 とみなす（L = Σp = 1） | 割らない。足した後は S > 0 になる |
| 窓 | 4週のまま | 記憶の長さの理由（カーフが窓から落ちて振り切れる）は比でも同じ |
| 画面の充足 | `target_sets` を「本人の実績の合計を配分どおりに割った量」にする（案1）★本人確認 | API の形・並び・棒が変わらない。合計の棒だけ意味を失う |
| 通し検証の帯 | 60〜145% のまま、比（q_r/p_r）で測る | 絶対量の帯は k に引きずられる（合計 105〜110%） |
| 通し検証の種目 | 今の38種目を明示した一覧で選ぶ | `runSim` はカタログ全件を選んでいる。目標を比にしても、足したプリセットが候補に入って数字が動く |

### 採らなかったもの

- **A：k を定数に書く。**プリセット非依存にはなるが、カタログと無関係な数字が残る。
  1.755 という値に意味が無くなり、達成率の合計も 105〜110% のまま
- **B：使う種目から k を出す。**使う種目を変えるたびに目標が動く。既定を最小限にすると
  k が BIG3 寄りになり、目標が膨らむ
- **カタログを今の38種目に凍結して k を数える。**新しいプリセットが目標に入らない理由が
  「凍結した日」になる。凍結した一覧の保守が増え、k の意味の無さは A と同じ
- **比にして、打ち切り（ΔL* ≥ 0 で止める）を残す。**試作で量が崩れた（次節）
- **1セットを寄与の合計で割って「1セット＝1」に正規化し、目標を p_r × 総セット数で持つ。**
  k が消え、打ち切りも残せる。ただしスクワットの大腿四頭の寄与が 1.0→0.4 になり、
  `regionShare` の較正（BIG3 の副次で埋まる区分を厚く置いた）をやり直すことになる。
  較正をそのままに試作し、寄与の単位（画面とエンジンが数える単位）で測ると形が崩れた
  （全身法でカーフが 0.38〜0.69、five_way は週2回以上で 1回 9セット前後）。正規化の単位では
  最適化しているので、手法が悪いのではなく、数える単位が2つになることが問題。
  画面の「セット」の意味も変わる

## 比で持つとは

```
p_r = w_r / Σ_{s∈R} w_s               w = regionShare、R = 目標のある区分

C_r = 窓 [E−27, E] に入る前日までの記録の刺激
    + 1週ぶんの軸・バリエーションの刺激（予測）
    + 割り振った補助の刺激（寄与 × セット数）            ← いまと同じ

S   = Σ_{r∈R} C_r
q_r = C_r / S                         S = 0 なら全区分 q_r = 0
ρ_r = q_r / p_r = C_r / (p_r · S)

loss_r = p_r × (1 − ρ_r)³              ρ_r < 1
       = p_r × α × (ρ_r − 1)³          ρ_r ≥ 1
L = Σ_{r∈R} loss_r
```

いまの損失は `T_r × g(C_r / 4T_r)`。4T_r を p_r·S（いま積んである総量を配分どおりに
割った量）に置き換え、全体を S/4 で割ったものに当たる。

**式から出てくる性質**

- **w を何倍しても p は動かない。**k・頻度・1回の量・窓の週数（4）が式から消える
- **重み p_r は T_r の役を引き継ぐ。**同じ相対的な遅れなら、配分の大小で有利不利が付かない
  （`TestAccessoryAllocator_DeltaLossIsNotBiasedByTargetSize` の性質）
- **1手で全区分の ρ が動く。**種目を足すと S が増え、触れていない区分の q も下がる。
  ΔL は効く区分の和ではなく、R の全区分で数え直す（1候補あたり 21区分）
- **目標0の区分は分母にも S にも入れない。**いまの「T>0 だけ追う」と同じ集合

**分母が小さいとき**

- **S = 0**：窓に記録が無く、先の回にも軸・バリエーションが無い（4週以上休んだあと、
  five_way・週1回で肩か腕の日に戻ってきた場合など。見通しはその1回だけで、軸が無い）。L = 1 から始め、最初の1手は ΔL が最小の候補を取る。
  配分どおりに広く効く種目が先に来る
- **S が小さい（新規の最初の週）**：S は1週ぶんの軸だけ。週3回の新規なら
  ベンチ・デッド・スクワットの3セットずつで S ≈ 24.3。配分の小さい区分の単関節種目は
  1本（3セット）で大きく超える。手計算（概算）で、サイドレイズは
  SideDelt の ρ が 0→4.2 になり ΔL ≈ +0.15。いまの絶対量では
  ρ = 3/(4×1.66) = 0.45 で ΔL ≈ −1.38 なので、損失を大きく減らす候補だった。
  比では、打ち切りがあるとこの種目は S が育つまで入らない。埋め切るなら後ろの枠に入る
- **補助が1本も無い週**（軸だけやった）：軸が積んだ区分の比が上がり、次の計画は
  それ以外の区分から埋める。いまと同じ
- **途中でやめる人・休んだ週**：総量が減っても形は変わらないので、取り戻そうとしない。
  いまも順序は相対的な遅れで決まり、枠はほぼ埋まるので、差は小さいと見込む（測っていない）
- **週頭**：窓はローリングの4週で、週の境目は無い（`CoverageWindowWeeks`）

## 補助の割り振りへの効き方

### 打ち切り条件（試作で測った）

`accessory_allocator.go` だけを `go test -overlay` で差し替え、通し検証を回した
（コミットしていない。試作のファイルも残していないので、PR 2 で測り直すときは「比で持つとは」の
式どおりに作り直す。α=1/4・β=0.9 は測り直していない）。
「形」は 実測の比 ÷ 配分（q_r/p_r）の最小と最大。帯 0.60〜1.45 の外に出た区分の数も数えた。

| 構成 | いま（絶対量） | 比＋打ち切り | 比＋埋め切り |
|---|---|---|---|
| 全身法 週2〜7回：1回の平均セット（予算12） | 11.5〜12.0 | 9.3〜11.8 | **12.0** |
| 全身法 週2〜7回：形 | 0.77〜1.39、外0 | 0.65〜1.44、外0 | 0.73〜1.41、外0 |
| upper_lower 週2〜7回：1回の平均セット | 11.1〜11.4 | 10.4〜10.7 | **12.0** |
| ppl 週2〜7回：1回の平均セット | 10.8〜11.6 | **7.7**〜10.2 | **12.0** |
| five_way 週2〜7回：1回の平均セット | 9.8〜10.1 | **4.7**〜6.6（0セットの日あり） | 11.7〜12.0 |
| five_way 週4〜7回：形 | 0.67〜1.31、外0 | 0.68〜1.31、外0 | 0.66〜**1.72**、外0〜2 |

**比＋打ち切りは量が崩れる。**比には「足りた」が無いので、形が整った時点で
どの1手も損失を増やし、枠を残して止まる。指数3の損失は ρ=1 で傾きが0なので、
整ったあとに足す1本は必ず損になる。five_way では肩の日が0セットになり、
周期がそこで止まった（`TestSimulation_SplitAlwaysProducesASession` が赤）。
`planning` の単体テストも、目標を数区分しか持たない入力で軒並み「補助が0件」になった。

**比＋埋め切りは、全身法と upper_lower・ppl でいまと同じ帯に収まる。**
five_way の週5・6回だけ、肩・腕の日を埋めた分が TRICEPS_LONG 1.62・SIDE_DELT 1.72 まで
超えた（`TestSimulation_SplitWeeklyTargetIsAttainable` が赤）。いまは打ち切りで
その日を 3〜9セットで終えていた分を、埋め切りが足している。
脚の日の容量不足（割り振りの設計書の容量の表、脚 0.37）の裏返しで、比で見ると他の日が相対的に超える、
という仮説を置く。PR 2 で α・β を測り直してから判断する。

### 1回の種目数の枠

枠は今までどおり `SessionVolume.Exercises()` から軸とバリエーションを引いた数
（`toHorizonSessions`）。埋め切るので、候補が尽きない限り毎回この数だけ入る。
**量は本人の設定だけで決まり、アプリは「何を」だけに答える**（CLAUDE.md の表）。

これは `2026-09-26-accessory-allocation-design.md` の
「枠が残っていても止まる（1回の量は上限であってノルマではない）」を覆す。
今日のリストは上から消していって好きなところでやめるキュー（D-116）で、
損失を増やす補助（埋め草）は貪欲の後半で入るので、ほとんどはその回のリストの後ろに並ぶ
（返す順の契約は「ΔL を貪欲に確定した順」。`TestSessionPlanner_AccessoriesComeBackInAStableOrder`）。
必ずではない。埋め草が S を増やすと、別の候補の ΔL が負に戻ることがある。試作（全身法と3分割、
週1〜7回）で今日のリスト961本のうち440本に埋め草が入り、そのうち10本は埋め草のあとに
損失を減らす補助が来た（並びは「減らす・埋め草・減らす」）。

### 軸・バリエーションの寄与

C_r と S の両方に入る。BIG3 の副次が厚い区分（臀筋・脊柱起立筋）は比で超え、
それ以外の区分が相対的に遅れる。いまと同じ向き。違いは、軸が超過区分に積んだ分も
S を通して他の区分の q を下げること。全区分に同じ割合で効くので、区分どうしの順位は変わらない。

### 選ばれる補助の変わり方（試作の実測。α・β を測り直すと動く）

- **新規の最初の数週。**いまは新規だと C が1週ぶんの軸しか無く、全区分が 4T に対して
  大きく遅れた状態から始まる。多区分に効く種目の ΔL の合計が大きくなるので、軸と同じ系統
  （front_squat・pause_squat・deficit_deadlift）が補助に入りやすい。比では軸が積んだ区分は
  既に超えているので、軸と重ならない種目が先に来る（解釈。下の表は実測）。
  全身法・週3回の最初の9回のうち2回：

  | 回 | 軸 | いま | 比＋埋め切り |
  |---|---|---|---|
  | 3 | squat | front_squat・pause_squat・calf_raise | dip・leg_press・lat_pulldown |
  | 8 | deadlift | barbell_row・deficit_deadlift・front_squat | barbell_row・rear_delt_fly・side_bend |

- **8週の通算（全身法）。**deficit_deadlift が減り（週3回 2→1、週5回 5→1）、
  leg_curl・hip_thrust・back_extension が増える。hammer_curl は週2〜7回で一度も出なくなる
  （いまは週3〜7回で出ている。前腕はデッドリフト・チンニングの副次で比が超える）
- **分割。**1回が毎回ほぼ12セットになる（いまは平均 9.8〜11.6）

なぜ deficit_deadlift が減るのかの切り分けはまだしていない。PR 2 で、
下の「数字で見るもの」の項目を構成ごとに並べて確かめる。

## 達成率・統計（`query/stats.go`）の表示

**いまの意味。**`TargetSet` は T_r（寄与×セット数の週あたり）、`DoneSet` は直近4週（当日を含む）の
同じ単位の週平均。区分ごとの「done / target」は「設定から見込んだ量に対してどれだけやったか」で、
合計の % は k がカタログ平均である分だけずれている（処方どおりで 105〜110%）。
単位は寄与の和なのに、画面は「セット」と書いている（週3回・4種目×3セットで合計 63.2）。

**比にすると。**T_r が無くなる。並び（埋まっていない順）は done_r/T_r と q_r/p_r が
同じ順になるので変わらない。注記「補助種目は、足りていない区分から選ばれます」もそのまま成り立つ。

**web で読んでいる箇所。**`History.tsx` の `WeeklySummary`（`weeklyTotal`）と `WeeklyVolume`
（`fillPercent`）、`weekly.test.ts`、`volume.test.ts`。開発用は `simulate.ts`（`rate`・`tone`・`outOfRange`）、
`chart.ts`、`RegionHeatmap.tsx`、`summary.ts`・`summary.test.ts`。`web/scripts/*-check.mjs` は
充足の数字を読んでいない（`volume-check.mjs` は休憩の音量）。

| 案 | 中身 | 変わるもの |
|---|---|---|
| **1（推奨）** | `target_sets` = p_r × Σ_r `done_sets`（本人の実績の合計を配分どおりに割った量）。型も名前もそのまま | 区分の行は「配分どおりなら何セットぶんか」に対する実績になる。合計の棒は常に100%になるので、`WeeklySummary` は合計の実績だけを出す。実績0なら target も0（`WeeklySummary` は既に出さない） |
| 2 | 割合を返す（`target_share`・`done_share`）。画面は「配分 8% / 実績 6%」 | フィールドを足し、web を先に出す。読み方が一番正直だが、変更が一番大きい |
| 3 | 表示だけ定数 k で絶対量を作る | 案 A を画面にだけ残す。採らない |

案1を推すのは、API の形・並び・区分の棒が変わらず、壊れるのが元から意味の薄かった
合計の % だけだから。古い web（PWA のキャッシュ）が新しいサーバーを読んでも
合計の棒が100%で止まるだけで落ちない。同じフィールド名で意味が変わることは、
PR の本文に書く。

## PR の分け方

1本に1つの判断。動作の変更と機械的な移動は混ぜない。

| PR | 中身 | 数字 |
|---|---|---|
| 1. 物差し | 通し検証の種目を明示の一覧にする。帯を比で測る。量の番人を足す | 動かない |
| 2. 入れ替え | 割り振り器の損失を比にし、埋め切る。α・β を測り直す。devsim の Target を案1の意味にする | **動く** |
| 3. 画面 | `Stats.WeeklyVolume` の `TargetSet` を案1にし、`WeeklySummary` を直す（本人の判断のあと） | 表示だけ動く |
| 4. 掃除 | `averageStimulusPerSet` と、`DefaultWeeklyTarget` の頻度・1回の量の引数を消す | 動かない |

4 を 3 の後にするのは、`stats.go` が `TargetSet` に週目標の値をそのまま出しているため。
先に消すと画面の数字が配分の生の値（5・10・4.5…）になる。

### PR 1：物差し（動かない）

- `runSim` の選択を `seed.Exercises()` の全件から、今日の38種目を書き並べた一覧に変える。
  `TestSimulation_EveryAccessoryGetsUsedInSomeSetup` も同じ一覧を回す
- `simResult.rate`・`outOfBand` を比で測る。目標は p_r × Σ_r 実測（週あたり）。
  補助1本ぶんの段（`setsPer1 / weeks`）はそのまま使う
- `TestSimulation_Report`・`SplitReport` に q_r/p_r を出す
- **量の番人** `TestSimulation_SessionsUseMostOfTheBudget`：週2回以上の構成ごとに 1回の平均セット数が
  予算の80%以上。比の帯は総量を見ないので、量が崩れても緑のまま通る。それを止める。
  週1回を外すのは帯と同じ理由（five_way 週1回はいまでも 9.0/12 = 75%）

検収：

- 全テストが緑。`TestSimulation_Report` の「実測」の列が変更前と1文字も変わらない
  （出力を diff する。選択の一覧が全件と同じ中身なので計画は動かない）
- いまの値で比の帯が全部収まることは試作で確かめた（全身法 週2〜7回 0.77〜1.39、
  upper_lower・ppl 週2〜7回と five_way 週4〜7回も外0）
- 量の番人の変異：試作の「比＋打ち切り」を入れて赤くなる（five_way 週3回で 4.7/12 = 39%）。
  いまの週2回以上の最小は five_way 週5〜7回の 9.8/12 = 82%

### PR 2：入れ替え（動く）

**先に書くテストと、赤くなる理由**

| テスト | 守ること | いまの実装で赤くなる理由 | 入れる変異（実装後） |
|---|---|---|---|
| `TestAccessoryAllocator_TargetScaleDoesNotChangeTheAllocation` | 週目標を定数倍しても割り振りが同じ | 4T と比べるので、倍率で打ち切りと最良が変わる（目標に近い区分を1つ置く） | p_r·S の代わりに 4T_r（定数倍が効く尺度）に戻す |
| `TestSessionPlanner_TargetScaleDoesNotMoveThePlan` | 実シード・`DefaultWeeklyTarget` を 0.9・1.1 倍して数週回しても、毎回の計画が同じ。**プリセットで k が動いても計画が動かない**ことを、k が週目標を一律に拡大縮小するだけ、という事実を使って固定する | 試作で、全身法・週3回は 0.9・1.1 倍とも2回目で計画が分かれた（同じ履歴からの1回ぶんでも24回中19〜21回が変わる） | 同上 |
| `TestAccessoryAllocator_FillsEverySlotWhileCandidatesRemain` | 候補が残る限り枠を埋める | 打ち切りで枠を残す | 手順2の打ち切りを戻す |
| `TestAccessoryAllocator_EmptySupplyStillAllocates` | 実績も軸も無い（S = 0）入力で NaN にならず埋まる | いまは C=0 でも割れないので緑（守り。赤は変異で確かめる） | S = 0 の扱いを外して 0 で割る |
| `TestAccessoryAllocator_NearBandWorksWhenEveryMoveCostsLoss` | 最良の ΔL* が正でも「ほぼ同じ」の帯が空にならず、多様性の絞り込みが働く | いまは ΔL* ≥ 0 で止まるので、その状態に来ない | 帯を β×ΔL* に戻す（帯が空になり落ちる） |

**作り直す割り振り器のテスト。**どれも「4週で 4T」を前提に実績を組んでいる。

- 比＋埋め切りの試作で赤：`TestAccessoryAllocator_DoesNotBlowUpRegionsAtTarget`。Oblique が130積んで
  あると、combo が足す3は比をほとんど動かさず、pure と β の帯に並ぶ。超過区分への罰は S が大きいほど
  薄まる。性質を「足りない区分の候補があれば、超えている区分の候補より先に入る」に書き直す。
  `TestAccessoryAllocator_TiesSpreadToTheSessionWithMoreFreeSlots` も赤。回1に入ったあと、
  同じ種目が回0にも埋め草として入る。「最初の1本は空き枠の多い回へ」を見る形に書き直す
- 緑のままだが、比でも同じ理由で通っているかを変異で確かめ直す：`DeficitIsPunishedMoreSteeply`・
  `DeltaLossIsNotBiasedByTargetSize`（変異は「重み p_r を外す」に読み替える）・
  `CubedLossFavorsTheMostBehindRegion`・`SecondaryContributionsCountToo`・`IgnoresRegionsWithoutATarget`。
  比＋打ち切りの試作では `SecondaryContributionsCountToo` ほか4本が「補助が0件」で落ちた。
  入力が数区分しかないと、形が整うのが早い
- **目標が1区分だけの入力は比では意味を失う。**q ≡ 1・p = 1 なので損失は常に0で、どの候補も
  ΔL = 0 の同点になる。`TiesSpread…`・`IgnoresRegionsWithoutATarget` はこの形で、
  後者は緑のままでも同点処理で通っているだけになる。目標は2区分以上で組む

**意味が変わる計画のテスト。**「目標を埋めていたら補助は出ない」「埋まった分だけ補助が減る」を
件数で見ているものは、埋め切ると件数が減らない。「埋まった区分の種目は、埋まっていない区分の種目より
後ろに並ぶ／入らない」に書き直す（比＋埋め切りの試作で赤になったもの）。

- `TestSessionPlanner_ResidualCarriesOverWithinTheWeek`・`SubtractsMainCoverageFromResidual`・
  `AccessoriesFollowTheRollingGap`・`DeselectedExercisesStillCountAsDone`・`RollingCoverageWindow`・
  `BodyweightSetsStillCountTowardCoverage`・`VariationCoverageFreesSlotsForOtherRegions`（`variation_lane_test.go`）
- `TestSessionPlanner_AxisIsEmptyWhenNoDeclaredFitsTheDay`・`NoSplitKeepsEverything`（`split_lane_test.go`）

**守り続けるもの**（このPRでも緑のまま）

- `TestSessionPlanner_PlanIsFixedForTheWholeDay`。比にしても履歴は前日までのまま（`Forecast` の
  `history := req.History.Before(req.Date)` は触らない）。分母 S が当日の記録で動かないことも同じ理由で守られる
- `TestSessionPlanner_ExerciseCountNeverExceedsBudget`、`TestSessionPlanner_IsDeterministic`
- `TestSimulation_SplitAlwaysProducesASession`（比＋打ち切りではここが落ちた）
- PR 1 の量の番人と、比の帯

**動く数字と確かめること**

- α∈{1/8,1/4,1/2}×β∈{0.8,0.9,0.95} を測り直す（前の設計書と同じ格子）
- five_way 週5・6回の超過が残るなら、対象外にするか（理由は容量）を本人に聞く。帯は広げない
- devsim の `RegionVolume.Target` を p_r × その週の実績の合計にし、`.claude/skills/dev-simulation/SKILL.md` の
  「許容帯は達成率…」を「q/p で 60〜145%」に書き換える。画面の帯（`simulate.ts`）はそのまま使える
- `go test ./internal/domain/training/seed/` の所要時間。試作（候補ごとに区分の表を写す素朴な実装）は
  9秒→34秒だった。2倍を超えるなら、S と L を1手に1回だけ数えて候補ごとの差分で出す

### PR 3：画面（表示だけ動く。本人の判断のあと）

- 案1なら `Stats.WeeklyVolume` の `TargetSet` を p_r × ΣDoneSet にする。`stats_test.go` の
  「`TargetSet == DefaultWeeklyTarget(...)`」を「区分の target の比が配分の比、合計が done の合計」に書き換える。
  変異：`TargetSet` を `DefaultWeeklyTarget` の値に戻して赤
- `WeeklySummary` は合計の実績だけを出す。`weekly.ts` の % と `weekly.test.ts` を合わせる
- 並び（埋まっていない順）は変わらないことをテストで固定する。変異：並びのキーを done にして赤
- `web/scripts/ui-check.mjs` を回す

### PR 4：掃除（動かない）

- `averageStimulusPerSet` を消す。`DefaultWeeklyTarget` から頻度・1回の量の引数を消す
  （呼び出しは `get_session.go`・`get_forecast.go`・`stats.go`・`devsim/simulator.go`・テスト）
- `WeeklyVolumeTarget` の doc コメント（「週あたり目標セット数」）を比に直す。型の改名は要るときに
- `TestDefaultWeeklyTarget_CoversEveryRegion`・`TestDefaultWeeklyTarget_ShareDoesNotMoveTotal` を消す。
  引数が無くなると何も守らない（緑のまま残すと「守っているつもり」になる）
- コメントの書き換え：`weekly_target.go`（k と「ゲートと同点の順序付けまで」）、`session_planner.go` の
  `PlanRequest`、`stats.go`、`CoverageWindowWeeks`（「目標も4週ぶんで比べる」）、
  `custom-exercises-design.md` の「週目標＝プリセットの一覧から計算」の行
- 検収：`TestSimulation_Report`・`SplitReport` の出力が PR 3 後と1文字も変わらない。
  割り振りが目標の倍率に依存しないので、k を消しても計画は動かない（PR 2 の
  `TargetScaleDoesNotMoveThePlan` がその前提を守っている）

## 数字で見るもの（dev-simulation の手順）

`TestSimulation_Report`・`SplitReport`（PR 1 で q/p が出る）と、`/api/dev/simulate`
（`growth=0`、頻度・分割を振る）で、PR 2 の前後を並べる。

| 指標 | 合格ライン |
|---|---|
| 形（q_r/p_r の最小・最大） | 全身法 週2〜7回、upper_lower・ppl 週2〜7回、five_way 週4〜7回で 0.60〜1.45（補助1本ぶんの段は問わない） |
| 1回の平均セット数 | 予算の80%以上（量の番人）。埋め切るなら全身法は予算ちょうど |
| 種目の出番 | `TestSimulation_EveryAccessoryGetsUsedInSomeSetup` が緑。構成ごとの使われた種目の数がいまより2以上減ったら理由を確かめる |
| 新規の立ち上がり | 週2回以上で、配分の小さい区分の単関節（side_raise か cable_side_raise、calf_raise、shrug、rear_delt_fly）が最初の2週のうちに出る |
| 尺度 | 週目標を 0.9・1.1 倍しても計画が同じ（テスト） |
| 所要時間 | seed のテストがいまの2倍以内 |

## 前提の訂正

- **「この表が効くのはゲートと同点の順序付けまで」（`DefaultWeeklyTarget` のコメント）は違う。**
  最良の組・β の帯・打ち切り（量）に効く。同点処理は週目標を読まない（「何が問題だったか」3）
- **週目標を比にしただけでは、`TestSimulation` はプリセットの追加で動く。**`runSim` は
  `seed.Exercises()` を全件選んでいるので、足したプリセットが候補に入る。選択を明示の一覧にする（PR 1）
- **`TestSimulation_EveryAccessoryGetsUsedInSomeSetup` はカタログ全件を回す。**似た効き方の
  マシンをまとめて足すと、出番の無い種目が出て赤になりうる。プリセットを足す PR で扱う
  （そのときの実態に合わせる。いま作らない）
- **「供給も割合に直して比べる」を、いまの打ち切りのまま入れると量が崩れる。**比には
  「足りた」が無い。打ち切りを外す（埋め切る）判断が要る。これは前の設計書の決定を覆す
- **週目標の「セット数」は寄与の和で、セット数ではない。**週3回・4種目×3セットで週目標の合計は
  63.2、実際のセット数は36。画面は「セット」と書いている
- **five_way の週2・3回は、いまの実装では帯に収まっている。**`TestSimulation_SplitReport` の
  「範囲外」は空。`TestSimulation_SplitWeeklyTargetIsAttainable` のコメントは対象外の理由を
  容量不足としているが、いまは赤にならない。PR 2 で帯を測り直すときに、外す理由を確かめ直す
- **`2026-09-26-custom-exercises-design.md` の「週目標＝プリセットの一覧から計算（今までどおり）」は
  この設計で置き換わる。**

## リスクと未決事項

### 本人の判断が要るもの

1. **枠を埋め切るか。**比を選ぶと「足りた」が無くなり、打ち切りを残すと量が崩れる（試作で確かめた）。
   埋め切るなら、前の設計書の「枠が残っていても止まる」を覆す。量は 1回の種目数（本人の設定）
   だけで決まる。試作では今日のリストの約半分（961本中440本）に損失を増やす補助が入り、
   ほとんどはリストの後ろに並んだ
2. **画面の充足をどう見せるか。**推奨は案1（`target_sets` を実績の合計の配分にする。合計の % は消す）
3. **five_way 週5・6回の超過**（埋め切りで肩・腕の日が超える）が α・β を測り直しても残る場合、
   受け入れるか、対象外にするか

### 実測しないと分からないもの

- 選ばれる補助の変化の理由（deficit_deadlift が減り、leg_curl・hip_thrust が増える）。試作は α・β が
  絶対量のときの値のまま。PR 2 で格子を測ってから、上の指標を構成ごとに並べる
- 区分ごとの過不足の出方。比では1区分の不足が他の全区分の相対的な超過として出る。
  候補が1つも無い区分（使う種目を最小限にした人）がどれくらい他を押し上げるか。
  q/p の最大を見る。devsim は全種目を選ぶ（`buildProgram`）ので、使う種目を絞るキーを足すか、
  通し検証の `simConfig.excluded` で絞って測る
- 所要時間（試作で約3.7倍）

### デプロイの日

計画は保存していないので、デプロイした日の途中に開き直すと、今日の補助が入れ替わりうる
（`PlanIsFixedForTheWholeDay` は同じ版の中の約束）。計画の導出を変える PR に共通の話で、
ジムの時間帯を避けて出す。

## 既存の利用者のデータ

**DB のスキーマ変更は要らない。**週目標の保存列は 0012（`0012_drop_weekly_target.sql`）で既に消してあり、
週目標は毎回、設定から導いている。記録（`set_logs`）・プログラム・使う種目は何も変わらない。
未来の計画は保存していない（`Plan` のコメント）ので、作り直すものも無い。

変わるのは、同じ記録から導く「今日の補助」と、画面の `target_sets` の意味（PR 3）だけ。
