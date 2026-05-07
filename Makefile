.PHONY: build test lint setup migrate-up migrate-down docker-up swag

build:
	go build ./...

test:
	go test -race ./...

lint:
	golangci-lint run

setup:
	cp api/.env.example api/.env
	cp processor/.env.example processor/.env

migrate-up:
	go run ./api migrate up

migrate-down:
	go run ./api migrate down

docker-up:
	docker compose up --build

swag:
	swag init -dir api -output docs
