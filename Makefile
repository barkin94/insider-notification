.PHONY: help infra up down build logs test lint swag install-tools \
        build-api build-processor test-api test-processor test-shared lint-api lint-processor

help: ## Show available commands
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  %-18s %s\n", $$1, $$2}'

infra: ## Start Postgres and Redis only (for local development)
	docker compose up -d postgres redis mock-ntfn-provider migrate-api migrate-processor

up: ## Start all services
	docker compose up -d

down: ## Stop all services
	docker compose down

build: ## Rebuild Docker images for api and processor
	docker compose build api processor

build-api: ## Rebuild Docker image for api
	docker compose build api

build-processor: ## Rebuild Docker image for processor
	docker compose build processor

logs: ## Tail logs for all services (Ctrl-C to stop)
	docker compose logs -f

test: ## Run all tests (requires Docker for testcontainers)
	cd api && GOWORK=off go test -race ./... && cd ..
	cd processor && GOWORK=off go test -race ./... && cd ..
	cd shared && GOWORK=off go test -race ./...

test-api: swag ## Run api tests
	cd api && GOWORK=off go test -race ./...

test-processor: ## Run processor tests
	cd processor && GOWORK=off go test -race ./...

test-shared: ## Run shared tests
	cd shared && GOWORK=off go test -race ./...

lint-fix: swag ## Run linter for all modules
	cd api && GOWORK=off golangci-lint run --fix ./... && cd ..
	cd processor && GOWORK=off golangci-lint run --fix ./... && cd ..
	cd shared && GOWORK=off golangci-lint run --fix ./... && cd ..

lint-api: swag ## Run linter for api
	cd api && GOWORK=off golangci-lint run ./...

lint-processor: ## Run linter for processor
	cd processor && GOWORK=off golangci-lint run ./...

swag: ## Regenerate Swagger docs (requires swag CLI)
	cd api && swag init -g cmd/main.go -o docs --parseDependency --parseInternal

install-tools: ## Install required CLI tools (swag, golangci-lint)
	go install github.com/swaggo/swag/cmd/swag@v1.16.6
	curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/HEAD/install.sh | sh -s -- -b $$(go env GOPATH)/bin v2.5.0
