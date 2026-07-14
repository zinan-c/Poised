APP_NAME := poised

.PHONY: run run-local test build fmt integration-postgres

run:
	go run ./cmd/poised -config configs/poised.example.json

run-local:
	bash scripts/run_local.sh

test:
	go test ./...

build:
	mkdir -p bin
	go build -o bin/$(APP_NAME) ./cmd/poised
	go build -o bin/$(APP_NAME)ctl ./cmd/poisedctl

fmt:
	gofmt -w cmd internal

integration-postgres:
	bash scripts/integration_postgres.sh
