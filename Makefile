.PHONY: run build up down up-dev up-prod logs tidy lint test health

# — Локальная разработка ──────────────────────────────────────────────
run:
	go run ./cmd/replicast

build:
	go build -o bin/replicast ./cmd/replicast

tidy:
	go mod tidy

lint:
	golangci-lint run ./...

test:
	go test ./... -v -race -count=1

# — Docker Compose ────────────────────────────────────────────────────
DC = docker compose -f deployments/docker-compose.yml

# Обычный запуск
up:
	$(DC) up --build -d

# Dev режим: text логи, debug уровень, быстрый outbox
up-dev:
	$(DC) -f deployments/docker-compose.dev.yml up --build -d

# Prod режим: json логи, warn уровень, ограничения памяти
up-prod:
	$(DC) -f deployments/docker-compose.prod.yml up --build -d

down:
	$(DC) down -v

logs:
	$(DC) logs -f --tail=100

# — Утилиты ───────────────────────────────────────────────────────────

# Проверка всех трёх узлов
health:
	@echo "=== node-a ===" && curl -sf http://localhost:8081/api/v1/health | jq .
	@echo "=== node-b ===" && curl -sf http://localhost:8082/api/v1/health | jq .
	@echo "=== node-c ===" && curl -sf http://localhost:8083/api/v1/health | jq .

# Node discovery
wellknown:
	@echo "=== node-a ===" && curl -sf http://localhost:8081/.well-known/replicast | jq .
	@echo "=== node-b ===" && curl -sf http://localhost:8082/.well-known/replicast | jq .
	@echo "=== node-c ===" && curl -sf http://localhost:8083/.well-known/replicast | jq .