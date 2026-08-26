GO             ?= go
VERSION        ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT         ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
LDFLAGS        := -X main.version=$(VERSION) -X main.commit=$(COMMIT)
COMPOSE        ?= docker compose

.DEFAULT_GOAL := help

.PHONY: help
help: ## Show this help
	@grep -hE '^[a-zA-Z_-]+:.*?## ' $(MAKEFILE_LIST) | awk 'BEGIN{FS=":.*?## "}{printf "  \033[36m%-18s\033[0m %s\n", $$1, $$2}'

.PHONY: build
build: ## Build both binaries into bin/
	$(GO) build -ldflags '$(LDFLAGS)' -o bin/api ./cmd/api
	$(GO) build -ldflags '$(LDFLAGS)' -o bin/worker ./cmd/worker

.PHONY: test
test: ## Run unit tests (no external dependencies required)
	$(GO) test ./...

.PHONY: lint
lint: ## Run go vet and the pinned golangci-lint
	$(GO) vet ./...
	$(GO) tool golangci-lint run

.PHONY: check
check: ## Run everything CI runs
	$(MAKE) build
	$(MAKE) lint
	$(MAKE) test

# --- Local environment -------------------------------------------------------

.PHONY: up
up: ## Start the full local stack and wait for every service to be healthy
	$(COMPOSE) up -d --build --wait

.PHONY: down
down: ## Stop the stack and remove its containers, networks, and volumes
	$(COMPOSE) down --remove-orphans --volumes

.PHONY: ps
ps: ## Show per-service health
	$(COMPOSE) ps

.PHONY: logs
logs: ## Follow application logs
	$(COMPOSE) logs -f api worker

.PHONY: test-integration
test-integration: ## Run integration tests against the running stack (needs `make up`)
	$(GO) test -tags=integration -count=1 ./tests/integration/...
