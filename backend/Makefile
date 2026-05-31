MIGRATIONS_DIR := db/migrations

# Override DATABASE_URL on the command line or set it in your environment.
DATABASE_URL ?= postgres://postgres:postgres@localhost:5432/playarena?sslmode=disable

.PHONY: build run test fmt tidy sqlc \
        docker-up docker-down docker-logs \
        migrate-up migrate-down migrate-down-all migrate-status migrate-force

# ── Build & Run ───────────────────────────────────────────────────────────────

build:
	go build -o bin/api ./cmd/api

run:
	go run ./cmd/api

test:
	go test -race ./...

fmt:
	go fmt ./...

tidy:
	go mod tidy

# ── SQLC code generation ──────────────────────────────────────────────────────

sqlc:
	sqlc generate

# ── Docker ────────────────────────────────────────────────────────────────────

docker-up:
	docker compose up -d

docker-down:
	docker compose down

docker-logs:
	docker compose logs -f postgres

# ── Migrations ────────────────────────────────────────────────────────────────
# Install golang-migrate (run once):
#   go install -tags 'postgres' github.com/golang-migrate/migrate/v4/cmd/migrate@latest

migrate-up:
	migrate -database "$(DATABASE_URL)" -path $(MIGRATIONS_DIR) up

migrate-down:
	migrate -database "$(DATABASE_URL)" -path $(MIGRATIONS_DIR) down 1

migrate-down-all:
	migrate -database "$(DATABASE_URL)" -path $(MIGRATIONS_DIR) down

migrate-status:
	migrate -database "$(DATABASE_URL)" -path $(MIGRATIONS_DIR) version

# Usage: make migrate-force VERSION=13
migrate-force:
	migrate -database "$(DATABASE_URL)" -path $(MIGRATIONS_DIR) force $(VERSION)
