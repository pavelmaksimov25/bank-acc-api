.PHONY: build run test tidy
build:
	CGO_ENABLED=0 go build -o bin/api ./cmd/api
test:
	go test ./...
tidy:
	go mod tidy
run:
	go run ./cmd/api
