# Lệnh tắt cho dev. Gõ `make help` để xem danh sách.
# Yêu cầu: Docker, Go (theo go.mod), sqlc v1.31.1, golang-migrate v4.20.1 (xem README).

# Nạp biến môi trường từ .env (nếu có) và export cho mọi lệnh con.
ifneq (,$(wildcard .env))
include .env
export
endif

MIGRATIONS := db/migration
# Database test cố định, chỉ lệnh test-db được phép xóa/tạo lại nó.
TEST_DB := livestream_test
# Dùng plugin `docker compose` nếu có, không thì dùng file chạy độc lập `docker-compose`.
COMPOSE := $(shell docker compose version >/dev/null 2>&1 && echo "docker compose" || echo "docker-compose")
PSQL_ADMIN := $(COMPOSE) exec -T postgres psql -U postgres -v ON_ERROR_STOP=1

.DEFAULT_GOAL := help

.PHONY: help
help: ## Liệt kê các lệnh
	@grep -hE '^[a-zA-Z_-]+:.*?## ' Makefile | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-14s\033[0m %s\n", $$1, $$2}'

## ---------- Môi trường ----------

.PHONY: up
up: ## Dựng Postgres, Redis, LiveKit (chờ tới khi sẵn sàng)
	$(COMPOSE) up -d --wait

.PHONY: down
down: ## Tắt môi trường (giữ dữ liệu)
	$(COMPOSE) down

.PHONY: destroy
destroy: ## Tắt môi trường và XÓA dữ liệu (volume)
	$(COMPOSE) down -v

.PHONY: logs
logs: ## Xem log các dịch vụ
	$(COMPOSE) logs -f

## ---------- Database ----------

.PHONY: migrate
migrate: ## Chạy migration lên DB dev
	migrate -path $(MIGRATIONS) -database "$(DATABASE_URL)" up

.PHONY: migrate-down
migrate-down: ## Lùi 1 migration trên DB dev
	migrate -path $(MIGRATIONS) -database "$(DATABASE_URL)" down 1

.PHONY: reset-db
reset-db: ## Xóa sạch DB dev rồi chạy lại migration
	$(PSQL_ADMIN) -c "DROP DATABASE IF EXISTS livestream WITH (FORCE)" -c "CREATE DATABASE livestream"
	$(MAKE) migrate

.PHONY: sqlc
sqlc: ## Sinh lại code Go từ db/query
	sqlc generate

## ---------- Chạy ----------

.PHONY: run
run: ## Chạy API server (đọc .env)
	go run .

## ---------- Kiểm tra ----------

.PHONY: test-db
test-db: ## Tạo lại DB test sạch và chạy migration
	$(PSQL_ADMIN) -c "DROP DATABASE IF EXISTS $(TEST_DB) WITH (FORCE)" -c "CREATE DATABASE $(TEST_DB)"
	migrate -path $(MIGRATIONS) -database "$(TEST_DATABASE_URL)" up

# -p 1: các package test dùng chung một DB nên phải chạy lần lượt.
# -race: bắt lỗi truy cập dữ liệu chung giữa các goroutine.
# -count=1: không dùng kết quả cache, luôn chạy thật trên DB.
.PHONY: test
test: test-db ## Chạy toàn bộ test (tạo lại DB test trước)
	go test -p 1 -race -count=1 ./...

.PHONY: fmt
fmt: ## Format code
	gofmt -w $$(git ls-files '*.go')

.PHONY: check
check: ## Các kiểm tra giống CI (trừ test): format, vet, build, sqlc không lệch
	@test -z "$$(gofmt -l $$(git ls-files '*.go'))" || (echo "Chưa gofmt:"; gofmt -l $$(git ls-files '*.go'); exit 1)
	go vet ./...
	go build ./...
	sqlc diff
