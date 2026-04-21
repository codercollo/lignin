SHELL := /bin/bash
.DEFAULT_GOAL := help

# ── Variables ──────────────────────────────────────────────────────────────────
BINARY_GATEWAY  := bin/gateway
BINARY_SCHED    := bin/scheduler
MODULE          := github.com/codercollo/lignin
GOFLAGS         := -trimpath -ldflags="-s -w"
COVERAGE_OUT    := coverage.out
COVERAGE_HTML   := coverage.html
MIGRATE         := migrate -path migrations -database "$(DATABASE_DSN)"

# ── Help ───────────────────────────────────────────────────────────────────────
.PHONY: help
help:
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-20s\033[0m %s\n", $$1, $$2}' | sort

# ── Build ──────────────────────────────────────────────────────────────────────
.PHONY: build
build: ## Build both binaries
	go build $(GOFLAGS) -o $(BINARY_GATEWAY)  ./cmd/gateway
	go build $(GOFLAGS) -o $(BINARY_SCHED)    ./cmd/scheduler

.PHONY: build-gateway
build-gateway: ## Build gateway binary only
	go build $(GOFLAGS) -o $(BINARY_GATEWAY) ./cmd/gateway

.PHONY: build-scheduler
build-scheduler: ## Build scheduler binary only
	go build $(GOFLAGS) -o $(BINARY_SCHED) ./cmd/scheduler

# ── Test ───────────────────────────────────────────────────────────────────────
.PHONY: test
test: ## Run unit tests (no integration)
	go test -race -count=1 -short ./...

.PHONY: test-integration
test-integration: ## Run all tests including integration (requires Docker)
	go test -race -count=1 -timeout 120s ./...

.PHONY: test-cover
test-cover: ## Run tests with coverage report
	go test -race -count=1 -coverprofile=$(COVERAGE_OUT) -covermode=atomic ./...
	go tool cover -html=$(COVERAGE_OUT) -o $(COVERAGE_HTML)
	@echo "Coverage report: $(COVERAGE_HTML)"

.PHONY: test-cover-check
test-cover-check: ## Fail if coverage drops below 80%
	go test -race -count=1 -coverprofile=$(COVERAGE_OUT) -covermode=atomic ./...
	@go tool cover -func=$(COVERAGE_OUT) | \
		awk '/^total:/ { gsub(/%/,""); if ($$3 < 80.0) { print "Coverage " $$3 "% is below 80%"; exit 1 } \
		else { print "Coverage " $$3 "% OK" } }'

# ── Lint ───────────────────────────────────────────────────────────────────────
.PHONY: lint
lint: ## Run golangci-lint
	golangci-lint run ./...

.PHONY: vet
vet: ## Run go vet
	go vet ./...

# ── Database ───────────────────────────────────────────────────────────────────
.PHONY: migrate-up
migrate-up: ## Apply all pending migrations
	$(MIGRATE) up

.PHONY: migrate-down
migrate-down: ## Roll back the last migration
	$(MIGRATE) down 1

.PHONY: migrate-status
migrate-status: ## Show migration status
	$(MIGRATE) version

.PHONY: migrate-create
migrate-create: ## Create a new migration: make migrate-create NAME=add_foo
	$(MIGRATE) create -ext sql -dir migrations -seq $(NAME)

# ── Docker ─────────────────────────────────────────────────────────────────────
.PHONY: docker-up
docker-up: ## Start local dev stack (postgres + redis)
	docker compose up -d postgres redis

.PHONY: docker-down
docker-down: ## Tear down local dev stack
	docker compose down -v

.PHONY: docker-build
docker-build: ## Build production Docker image
	docker build -t lignin:dev .

# ── Code generation ────────────────────────────────────────────────────────────
.PHONY: generate
generate: ## Run all go:generate directives (mocks etc.)
	go generate ./...

# ── Seed ───────────────────────────────────────────────────────────────────────
.PHONY: seed
seed: ## Insert development fixture data
	go run ./scripts/seed.go

# ── Tidy ───────────────────────────────────────────────────────────────────────
.PHONY: tidy
tidy: ## Tidy go.mod and go.sum
	go mod tidy

.PHONY: clean
clean: ## Remove build artefacts
	rm -rf bin/ $(COVERAGE_OUT) $(COVERAGE_HTML)