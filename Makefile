.PHONY: up down logs test test-integration test-all build run

## up: start all services and wait until healthchecks pass
up:
	docker compose --env-file ./configs/.env.development up -d --wait

## down: stop and remove containers (data volumes are preserved)
down:
	docker compose --env-file ./configs/.env.development down

## down-clean: stop containers AND delete all volumes (fresh slate)
down-clean:
	docker compose --env-file ./configs/.env.development down -v

## logs: tail all container logs
logs:
	docker compose --env-file ./configs/.env.development logs -f

## test: run unit tests only (no database required)
test:
	go test -short -v ./internal/...

## test-integration: run integration tests against the running docker services
test-integration:
    TEST_POSTGRES_DSN=1 \
    TEST_POSTGRES_HOST=localhost \
    TEST_POSTGRES_USER=postgres \
    TEST_POSTGRES_PASSWORD='$$nadeem03' \
    TEST_POSTGRES_DB=streamtogether \
    go test -v ./internal/database/...

## test-all: start services, run every test, leave services running
test-all: up test test-integration

## build: compile the server binary to bin/server
build:
	go build -o bin/server ./cmd/server

## run: run the server directly (services must be up)
run:
	go run ./cmd/server