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
node scripts/adversarial-check.mjs   # 使い方はファイル冒頭
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
| `src/features/` | 今日・最初の設定 | — |
| `src/app/` | 画面の骨組みと状態表示 | — |

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
- **履歴・設定は落としてある（D-120）。**サーバーの `/api/stats` と `/api/program`
  は残っているので、戻すときは呼び出しを足すだけ
- **`src/api/types.ts` は `../internal/presentation/httpapi/dto.go` と対。**
  サーバーは `DisallowUnknownFields` なので、余分なフィールドを送ると 400 で
  弾かれ、待ち行列がそれを捨てる。片方だけ変えない
