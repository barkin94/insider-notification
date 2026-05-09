.PHONY: help infra up down build logs test lint

help: ## Show available commands
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  %-14s %s\n", $$1, $$2}'

infra: ## Start Postgres and Redis only (for local development)
	docker compose up -d postgres redis

up: ## Run migrations then start all services
	docker compose up -d

down: ## Stop all services
	docker compose down

build: ## Rebuild Docker images for api and processor
	docker compose build api processor

logs: ## Tail logs for all services (Ctrl-C to stop)
	docker compose logs -f

test: ## Run all tests (requires Docker for testcontainers)
	go test -race ./...

lint: ## Run linter (requires golangci-lint)
	golangci-lint run
