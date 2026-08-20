.PHONY: build run test up down logs smoke tidy migrate-create

build:
	CGO_ENABLED=0 go build -o bin/api ./cmd/api

run:
	go run ./cmd/api

test:
	go test ./... -race

up:
	docker compose up --build -d

down:
	docker compose down -v

logs:
	docker compose logs -f api

smoke:
	./scripts/smoke.sh

tidy:
	go mod tidy

migrate-create:
	goose -dir migrations create $(name) sql
