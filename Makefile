.PHONY: help infra up down build logs test lint swag install-tools \
        build-api build-processor build-deliveryscheduler \
        test-api test-processor test-deliveryscheduler test-shared \
        lint-api lint-processor lint-deliveryscheduler lint-shared

help: ## Show available commands
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  %-18s %s\n", $$1, $$2}'

infra: ## Start Postgres, Redis, NATS and Mockoon only (for local development)
	docker compose up -d postgres redis nats mock-ntfn-provider migrate-api migrate-deliveryscheduler

instrumentation: ## Instrumentation related services (for local development)
	docker compose up -d grafana prometheus loki tempo otel-collector

up: ## Start all services
	docker compose up -d

down: ## Stop all services
	docker compose down

build: ## Rebuild Docker images for api, processor, and deliveryscheduler
	docker compose build api processor deliveryscheduler

build-api: ## Rebuild Docker image for api
	docker compose build api

build-processor: ## Rebuild Docker image for processor
	docker compose build processor

build-deliveryscheduler: ## Rebuild Docker image for deliveryscheduler
	docker compose build deliveryscheduler

logs: ## Tail logs for all services (Ctrl-C to stop)
	docker compose logs -f

test: ## Run all tests (requires Docker for testcontainers)
	cd api && go test -race ./... && cd ..
	cd processor && go test -race ./... && cd ..
	cd deliveryscheduler && go test -race ./... && cd ..
	cd shared && go test -race ./...

test-api: swag ## Run api tests
	cd api && go test -race ./...

test-processor: ## Run processor tests
	cd processor && go test -race ./...

test-deliveryscheduler: ## Run deliveryscheduler tests
	cd deliveryscheduler && go test -race ./...

test-shared: ## Run shared tests
	cd shared && go test -race ./...

lint-fix: swag ## Run linter for all modules
	cd api && golangci-lint run --fix ./... && cd ..
	cd processor && golangci-lint run --fix ./... && cd ..
	cd deliveryscheduler && golangci-lint run --fix ./... && cd ..
	cd shared && golangci-lint run --fix ./... && cd ..

lint-api: swag ## Run linter for api
	cd api && golangci-lint run ./...

lint-processor: ## Run linter for processor
	cd processor && golangci-lint run ./...

lint-deliveryscheduler: ## Run linter for deliveryscheduler
	cd deliveryscheduler && golangci-lint run ./...

lint-shared: ## Run linter for shared
	cd shared && golangci-lint run ./...

swag: ## Regenerate Swagger docs (requires swag CLI)
	cd api && swag init -g cmd/main.go -o docs --parseDependency --parseInternal

install-tools: ## Install required CLI tools (swag, golangci-lint)
	go install github.com/swaggo/swag/cmd/swag@v1.16.6
	curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/HEAD/install.sh | sh -s -- -b $$(go env GOPATH)/bin v2.5.0
