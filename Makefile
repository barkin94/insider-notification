.PHONY: help infra up down build logs test lint swag install-tools \
        build-api build-processor build-retryscheduler build-deliveryscheduler \
        test-api test-processor test-retryscheduler test-deliveryscheduler test-shared \
        lint-api lint-processor lint-retryscheduler lint-deliveryscheduler lint-shared

help: ## Show available commands
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  %-18s %s\n", $$1, $$2}'

infra: ## Start Postgres and Redis only (for local development)
	docker compose up -d postgres redis mock-ntfn-provider migrate-api migrate-retryscheduler migrate-deliveryscheduler

up: ## Start all services
	docker compose up -d

down: ## Stop all services
	docker compose down

build: ## Rebuild Docker images for api, processor, retryscheduler, and deliveryscheduler
	docker compose build api processor retryscheduler deliveryscheduler

build-api: ## Rebuild Docker image for api
	docker compose build api

build-processor: ## Rebuild Docker image for processor
	docker compose build processor

build-retryscheduler: ## Rebuild Docker image for retryscheduler
	docker compose build retryscheduler

build-deliveryscheduler: ## Rebuild Docker image for deliveryscheduler
	docker compose build deliveryscheduler

logs: ## Tail logs for all services (Ctrl-C to stop)
	docker compose logs -f

test: ## Run all tests (requires Docker for testcontainers)
	cd api && GOWORK=off go test -race ./... && cd ..
	cd processor && GOWORK=off go test -race ./... && cd ..
	cd retryscheduler && GOWORK=off go test -race ./... && cd ..
	cd deliveryscheduler && GOWORK=off go test -race ./... && cd ..
	cd shared && GOWORK=off go test -race ./...

test-api: swag ## Run api tests
	cd api && GOWORK=off go test -race ./...

test-processor: ## Run processor tests
	cd processor && GOWORK=off go test -race ./...

test-retryscheduler: ## Run retryscheduler tests
	cd retryscheduler && GOWORK=off go test -race ./...

test-deliveryscheduler: ## Run deliveryscheduler tests
	cd deliveryscheduler && GOWORK=off go test -race ./...

test-shared: ## Run shared tests
	cd shared && GOWORK=off go test -race ./...

lint-fix: swag ## Run linter for all modules
	cd api && GOWORK=off golangci-lint run --fix ./... && cd ..
	cd processor && GOWORK=off golangci-lint run --fix ./... && cd ..
	cd retryscheduler && GOWORK=off golangci-lint run --fix ./... && cd ..
	cd deliveryscheduler && GOWORK=off golangci-lint run --fix ./... && cd ..
	cd shared && GOWORK=off golangci-lint run --fix ./... && cd ..

lint-api: swag ## Run linter for api
	cd api && GOWORK=off golangci-lint run ./...

lint-processor: ## Run linter for processor
	cd processor && GOWORK=off golangci-lint run ./...

lint-retryscheduler: ## Run linter for retryscheduler
	cd retryscheduler && GOWORK=off golangci-lint run ./...

lint-deliveryscheduler: ## Run linter for deliveryscheduler
	cd deliveryscheduler && GOWORK=off golangci-lint run ./...

lint-shared: ## Run linter for shared
	cd shared && GOWORK=off golangci-lint run ./...

swag: ## Regenerate Swagger docs (requires swag CLI)
	cd api && swag init -g cmd/main.go -o docs --parseDependency --parseInternal

install-tools: ## Install required CLI tools (swag, golangci-lint)
	go install github.com/swaggo/swag/cmd/swag@v1.16.6
	curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/HEAD/install.sh | sh -s -- -b $$(go env GOPATH)/bin v2.5.0
