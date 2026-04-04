.PHONY: build run dev test clean migrate frontend

# Go build
BUILD_DIR := build
BINARY := $(BUILD_DIR)/gitwise

build: frontend
	@mkdir -p $(BUILD_DIR)
	go build -o $(BINARY) ./cmd/gitwise

run: build
	$(BINARY)

# Development: run backend and frontend concurrently
dev:
	@echo "Starting backend..."
	@go run ./cmd/gitwise &
	@echo "Starting frontend dev server..."
	@cd web && npm run dev

test:
	go test ./... -v -race

test-short:
	go test ./... -short

lint:
	golangci-lint run ./...

# Frontend
frontend:
	cd web && npm run build

frontend-dev:
	cd web && npm run dev

# Database
migrate:
	@echo "Running migrations..."
	@go run ./cmd/gitwise migrate

# Docker
docker-up:
	docker compose up -d

docker-down:
	docker compose down

# Clean
clean:
	rm -rf $(BUILD_DIR)
	rm -rf web/dist
