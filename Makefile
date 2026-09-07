DATABASE_URL ?= postgres://liftplan:liftplan@127.0.0.1:5433/liftplan

# 開発用の固定トークン。本番は Secret Manager から渡す。
AUTH_TOKEN ?= dev-token-0123456789abcdef0123456789ab

# 画面（pnpm dev）のオリジン。未設定だとサーバーが起動しない。
ALLOWED_ORIGINS ?= http://localhost:5173

.PHONY: test test-db db-up db-down run docker-run releases

# DB を必要としないテスト。ドメインの検証はここで完結する。
test:
	go test ./...

# Postgres 実装を含む全テスト。
test-db: db-up
	TEST_DATABASE_URL='$(DATABASE_URL)' go test ./...

db-up:
	docker compose up -d --wait db

db-down:
	docker compose down -v

run: db-up
	DATABASE_URL='$(DATABASE_URL)' AUTH_TOKEN='$(AUTH_TOKEN)' \
		ALLOWED_ORIGINS='$(ALLOWED_ORIGINS)' go run ./cmd/api

# 画面。サーバーとは別のターミナルで動かす。
.PHONY: web
web:
	cd web && pnpm dev

# 本番と同じイメージで動かす。
docker-run: db-up
	docker build -t liftplan-server .
	docker run --rm -p 8080:8080 --network host \
		-e DATABASE_URL='$(DATABASE_URL)' -e AUTH_TOKEN='$(AUTH_TOKEN)' \
		-e ALLOWED_ORIGINS='$(ALLOWED_ORIGINS)' \
		liftplan-server

# 何がいつ出たかを一覧する。
releases:
	@./scripts/releases.sh
