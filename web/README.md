# liftplan の画面

React 19 + TypeScript + Vite + Tailwind CSS v4。Cloudflare Workers から配る。
API は別オリジン（Cloud Run）にあり、CORS で叩く。

設計は `../docs/superpowers/specs/2026-09-07-pwa-client-design.md`、
サーバーと分けた理由は `../docs/decisions.md` の D-119。

## 動かす

```bash
# 1. サーバー（別のターミナル）
cd .. && ALLOWED_ORIGINS=http://localhost:5173 make run

# 2. 画面
cp .env.example .env.local
pnpm install
pnpm dev
```

**開発でも proxy を挟まない。**`/api` を Vite の proxy で同一オリジンに見せると、
CORS を一度も通らないまま開発が終わり、設定漏れが本番で初めて出る。
本番と同じくクロスオリジンで叩き、手元で落ちるようにしてある。

サーバー側に `ALLOWED_ORIGINS=http://localhost:5173` を渡すのを忘れると、
起動しないか、画面から叩けない。**どちらも黙って壊れるより早く気づく形**。

## 検収

```bash
pnpm typecheck
pnpm test
pnpm build
```

加えて、**記録が消える経路は実機で通す**。

```bash
node scripts/adversarial-check.mjs   # 記録が消える経路。使い方はファイル冒頭
node scripts/ui-check.mjs           # 休憩タイマーと記録シートの初期値
node scripts/nav-check.mjs          # 画面の行き来と戻るジェスチャー
```

単体テストでは踏めない経路（Service Worker・IndexedDB の再読み込み・
オフライン復帰）を本番ビルドで検査する。**dev サーバーでは意味がない**
（SW が無効なので、圏外で再読み込みする経路を検査できない）。

実際にこれで既存バグを2件見つけている（SW が初回訪問で登録されない、
電波が戻ってもメニューが空のまま）。

## 分割

| 場所 | 役割 | 何を知らないか |
|---|---|---|
| `src/api/` | fetch と契約の型 | 画面を知らない |
| `src/outbox/` | 送信の待ち行列（IndexedDB） | **React も fetch も知らない** |
| `src/domain/` | 日付・セットの整形・区分の日本語 | React も fetch も知らない |
| `src/storage/` | localStorage（トークン） | 画面を知らない |
| `src/features/` | 今日・履歴・最初の設定 | — |
| `src/ui/` | 画面をまたぐ部品（Button / Card / Field / Stepper） | 画面の事情を知らない |
| `src/app/` | 画面の骨組みと状態表示 | — |

**`features/history/` はまだどこからも呼ばれていない。**部品だけがある状態で、
どこから行くかはナビゲーションの設計を待っている（D-127）。

**プログラムの設定に画面は無い。**週に通う回数・伸ばしたい種目・重点種目・
使う種目・週の目標セット数は `features/today/ProgramSettings.tsx` として
「今日」の末尾に畳んである。

**`outbox/` が React も fetch も知らないのが要点。**ジムで一番壊れてほしくない
ロジック（再送・冪等ID・修正時の削除→再投入・4xx の破棄）を、DOM も
ネットワークも立てずに検査できる。送信関数は引数で受け取る。

## 触るときに気をつけること

- **DB名・ストア名・localStorage のキーを変えない。**送りきれていない記録が読めなくなる
- **`sw.ts` で `skipWaiting()` を自動で呼ばない。**記録シートを開いている最中に
  画面が差し替わると、入力中の値が消える。待機中の新版は全てのクライアントが
  閉じたときに有効になる
- **SW の登録は `main.tsx` に置く。**画面の中の部品から登録すると、その部品が
  描かれるまで登録されない（実際、更新通知の部品に置いていたときは、トークンを
  入れるまで登録されなかった）
- **プログラムの設定は1フィールドずつの口へ送る。**`PUT /api/program` は
  全置換で、フィールドを1つ並べ忘れると欠けたまま届く。
  `DisallowUnknownFields` が弾くのは**余分な**フィールドだけで、欠落は
  素通りする——400 ではなく 204 が返り、設定が黙って初期値に戻る。
  `PUT /api/program/{frequency,declared,focus,selected,target}` を使うこと
- **履歴の画面は読むだけ。**`stats` と `days` を props で受け取り、自分では
  取りに行かない（取るのは `useStats`）。書き込みを足すと、そのぶん
  「契約ずれ → 400 → 待ち行列が破棄 → 記録が消える」経路が増える
  （D-119・D-127）
- **画面の一番下に何かを足したら、固定バー2本に潜っていないか実機で測る。**
  `body` の `padding-bottom` は 92px で状態バー1本ぶん。休憩タイマーが
  増えたときに広げられておらず、実際に数px潜る（D-127）
- **`src/api/types.ts` は `../internal/presentation/httpapi/dto.go` と対。**
  サーバーは `DisallowUnknownFields` なので、余分なフィールドを送ると 400 で
  弾かれ、待ち行列がそれを捨てる。片方だけ変えない
