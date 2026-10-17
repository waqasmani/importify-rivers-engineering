# REA Importify developer and operator commands. Run `make help` for the list.
SHELL := /usr/bin/env bash
.DEFAULT_GOAL := help

COMPOSE := docker compose -f deploy/docker-compose.yml
TEST_DATABASE_URL ?= postgres://rea:rea@127.0.0.1:5432/rea?sslmode=disable
VERSION ?= $(shell node -p "require('./package.json').version" 2>/dev/null || echo dev)
SQLC := $(shell command -v sqlc 2>/dev/null || echo $(HOME)/go/bin/sqlc)

.PHONY: help
help: ## Show this help
	@grep -E '^[a-zA-Z0-9_-]+:.*?## ' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-18s\033[0m %s\n", $$1, $$2}'

# ---- setup ----
.PHONY: deps
deps: ## Install Go modules and JavaScript dependencies
	go mod download
	pnpm install --frozen-lockfile

.PHONY: generate
generate: ## Regenerate sqlc code, the OpenAPI document and TypeScript API types
	$(SQLC) generate
	REA_UPDATE_OPENAPI=1 go test ./services/internal/httpapi -run TestCommittedOpenAPIIsCurrent -count=1
	pnpm api-types

# ---- build ----
.PHONY: build build-go build-web build-extension
build: build-go build-web build-extension ## Build API, worker, dashboard and extension

build-go: ## Build bin/rea-api and bin/rea-worker
	CGO_ENABLED=0 go build -trimpath -tags timetzdata -ldflags "-s -w -X main.version=$(VERSION)" -o bin/rea-api ./services/api
	CGO_ENABLED=0 go build -trimpath -tags timetzdata -ldflags "-s -w -X main.version=$(VERSION)" -o bin/rea-worker ./services/worker

build-web: ## Build the dashboard into apps/web/dist
	pnpm --filter @rea/web build

build-extension: ## Build the browser extension into apps/extension/dist
	pnpm --filter @rea/extension build

# ---- quality ----
.PHONY: lint fmt-check vet typecheck
lint: fmt-check vet typecheck ## gofmt check, go vet, migration lint and TypeScript checks
	go test ./services/internal/migrate -run 'TestMigrationLint' -count=1

fmt-check:
	@test -z "$$(gofmt -l services db)" || (gofmt -l services db; echo "run gofmt -w services db"; exit 1)

vet:
	go vet ./...

typecheck:
	pnpm -r typecheck

# ---- tests ----
.PHONY: test test-go test-web test-extractor test-extension e2e
test: test-go test-web test-extractor test-extension ## Run all unit and integration tests (needs PostgreSQL, see docs/development.md)

test-go: ## Go unit + database integration + API contract tests
	REA_TEST_DATABASE_URL="$(TEST_DATABASE_URL)" REA_REQUIRE_DB_TESTS=1 go test -count=1 ./...

test-web: ## Vue component tests
	pnpm --filter @rea/web test

test-extractor: ## Extraction package tests: adapters, rules engine, every marketplace fixture
	pnpm --filter @rea/extractor test

test-extension: ## Extension unit tests
	pnpm --filter @rea/extension test

e2e: build-web build-extension ## Browser end-to-end tests against a real API (needs PostgreSQL)
	pnpm --filter @rea/web test:e2e
	pnpm --filter @rea/extension test:e2e

# ---- local development ----
.PHONY: dev-api dev-web
dev-api: ## Run the API on :8080 against a local database (deploy/scripts/dev-api.sh)
	deploy/scripts/dev-api.sh rea_dev 8080

dev-web: ## Run the dashboard dev server on :5173 (proxies /api to :8080)
	pnpm --filter @rea/web dev

# ---- Docker Compose deployment ----
.PHONY: env up down logs ps backup verify-backup restore images
env: ## Create deploy/.env with generated secrets (DOMAIN=rea.example.com for HTTPS)
	deploy/scripts/init-env.sh $(DOMAIN)

images: ## Build the api and web images
	$(COMPOSE) build

up: ## Start the stack (db, api, web)
	$(COMPOSE) up -d --build

down: ## Stop the stack (data volumes are kept)
	$(COMPOSE) down

logs: ## Follow logs
	$(COMPOSE) logs -f --tail=100

ps: ## Show service status
	$(COMPOSE) ps

backup: ## Back up database and data directory into deploy/backups
	deploy/scripts/backup.sh

verify-backup: ## Restore a backup into a temporary database and check it (BACKUP=deploy/backups/rea-...)
	deploy/scripts/verify-backup.sh $(BACKUP)

restore: ## Restore a backup (BACKUP=deploy/backups/rea-...); destructive, asks for confirmation
	deploy/scripts/restore.sh $(BACKUP)

.PHONY: clean
clean: ## Remove build output
	rm -rf bin apps/web/dist apps/extension/dist
