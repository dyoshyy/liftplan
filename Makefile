DATABASE_URL ?= postgres://liftplan:liftplan@127.0.0.1:5433/liftplan

.PHONY: test test-db db-up db-down run

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
	DATABASE_URL='$(DATABASE_URL)' go run ./cmd/api
