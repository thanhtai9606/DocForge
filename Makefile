.DEFAULT_GOAL := help

# DocForge — Makefile (pattern from ai-traiding)
# Host binaries go to bin/. Docker images are multi-stage (Go builds inside Dockerfile).

BIN_DIR := bin
API_MOD := apps/api
CDOM_MOD := packages/cdom
WEB := apps/web

API_BINARY := $(BIN_DIR)/api
WORKER_BINARY := $(BIN_DIR)/worker

DOCKER_BUILDKIT ?= 1
export DOCKER_BUILDKIT
GOOS_DOCKER ?= linux
GOARCH_DOCKER ?= $(shell go env GOARCH 2>/dev/null || echo amd64)
DOCKER_REGISTRY ?= ghcr.io
GHCR_OWNER ?= thanhtai9606
IMAGE_TAG ?= $(shell git rev-parse --short HEAD 2>/dev/null | tr '/:' '--')
ifeq ($(strip $(IMAGE_TAG)),)
IMAGE_TAG := latest
endif

API_IMAGE := $(DOCKER_REGISTRY)/$(GHCR_OWNER)/docforge-api
WORKER_IMAGE := $(DOCKER_REGISTRY)/$(GHCR_OWNER)/docforge-worker

ENV_FILE ?= apps/api/configs/local.env
ENV_EXAMPLE := apps/api/configs/local.env.example
COMPOSE_INFRA := docker compose -f deployments/docker-compose.yml
COMPOSE_API := docker compose -f deployments/docker-compose.yml -f deployments/docker-compose.api.yml

.PHONY: help \
	api worker build build-go build-api build-worker build-linux-go build-linux-all build-all \
	build-web build-frontend \
	run-api run-worker run-api-memory run-web run-all \
	infra infra-down infra-logs up down up-api down-api \
	test test-go test-cdom test-api test-web fmt vet lint-web tidy tidy-go clean env codegraph \
	docker-build-all docker-package docker-push-all docker-login \
	docker-build-api docker-build-worker docker-push-api docker-push-worker

## help: Show Makefile targets
help:
	@echo "DocForge — Makefile targets (run from repo root):"
	@echo ""
	@grep -hE '^## ' $(MAKEFILE_LIST) | sed 's/^## /  make /' | column -t -s ':' 2>/dev/null || grep -hE '^## ' $(MAKEFILE_LIST) | sed 's/^## /  make /'

# --- Aliases ---

## api: Alias — build bin/api
api: build-api

## worker: Alias — build bin/worker
worker: build-worker

# --- Build Go ---

## build-go: Build API + worker into bin/
build-go: build-api build-worker

build-api:
	@mkdir -p $(BIN_DIR)
	go build -trimpath -ldflags="-s -w" -o $(API_BINARY) ./$(API_MOD)/cmd/api

build-worker:
	@mkdir -p $(BIN_DIR)
	go build -trimpath -ldflags="-s -w" -o $(WORKER_BINARY) ./$(API_MOD)/cmd/worker

# --- Build frontend ---

## build-web: npm ci + Vite production build
build-web build-frontend:
	cd $(WEB) && npm ci && npm run build

# --- Aggregate ---

## build: Go binaries + web production build
build: build-go build-web

## build-linux-go: Linux API + worker binaries in bin/ (optional; Dockerfiles build themselves)
build-linux-go:
	@mkdir -p $(BIN_DIR)
	CGO_ENABLED=0 GOOS=$(GOOS_DOCKER) GOARCH=$(GOARCH_DOCKER) go build -trimpath -ldflags="-s -w" -o $(API_BINARY) ./$(API_MOD)/cmd/api
	CGO_ENABLED=0 GOOS=$(GOOS_DOCKER) GOARCH=$(GOARCH_DOCKER) go build -trimpath -ldflags="-s -w" -o $(WORKER_BINARY) ./$(API_MOD)/cmd/worker

## build-linux-all: Linux Go binaries + web
build-linux-all: build-linux-go build-web

## build-all: Alias for build-linux-all
build-all: build-linux-all

# --- Local run ---

_run_env = set -a && . $(ENV_FILE) && set +a

## env: Copy local.env.example → apps/api/configs/local.env if missing
env:
	@test -f $(ENV_FILE) || (cp $(ENV_EXAMPLE) $(ENV_FILE) && echo "Created $(ENV_FILE) — edit secrets before production.")

## run-api: Build + run HTTP API on port 8080 (needs infra or matching env)
run-api: env
	@$(MAKE) build-api
	@bash -c '$(_run_env); exec ./$(API_BINARY)'

## run-worker: Build + run RabbitMQ processing worker
run-worker: env
	@$(MAKE) build-worker
	@bash -c '$(_run_env); exec ./$(WORKER_BINARY)'

## run-api-memory: API with in-memory adapters (no Postgres/Redis/RabbitMQ/MinIO)
run-api-memory:
	@$(MAKE) build-api
	@DOCFORGE_USE_MEMORY=1 exec ./$(API_BINARY)

## run-web: Vite dev server (proxies /api to port 8080)
run-web:
	@test -x $(WEB)/node_modules/.bin/vite || (cd $(WEB) && npm install)
	cd $(WEB) && npm run dev

## run-all: Infra + API + worker in one terminal (Ctrl+C stops processes; logs in .run/)
run-all: env infra
	@$(MAKE) build-go
	@bash scripts/run-all.sh

# --- Compose (MVP infra) ---

## infra: Start Postgres + Redis + RabbitMQ + MinIO
infra:
	$(COMPOSE_INFRA) up -d

## infra-down: Stop infra compose
infra-down:
	$(COMPOSE_INFRA) down

## infra-logs: Follow infra logs
infra-logs:
	$(COMPOSE_INFRA) logs -f

## up: Alias infra
up: infra

## down: Alias infra-down
down: infra-down

## up-api: Infra + API container (GHCR or locally tagged image)
up-api:
	GHCR_OWNER=$(GHCR_OWNER) API_IMAGE_TAG=$(IMAGE_TAG) $(COMPOSE_API) up -d

## down-api: Stop infra + API overlay
down-api:
	GHCR_OWNER=$(GHCR_OWNER) API_IMAGE_TAG=$(IMAGE_TAG) $(COMPOSE_API) down

# --- Quality ---

## test: CDOM + API Go tests
test test-go:
	./scripts/test.sh

## test-cdom: go test packages/cdom
test-cdom:
	go test ./$(CDOM_MOD)/... -count=1

## test-api: go test apps/api
test-api:
	go test ./$(API_MOD)/... -count=1

## test-web: Typecheck + oxlint
test-web:
	cd $(WEB) && npx tsc -b --pretty false && npm run lint

## fmt: go fmt
fmt:
	go fmt ./$(CDOM_MOD)/... ./$(API_MOD)/...

## vet: go vet
vet:
	go vet ./$(CDOM_MOD)/... ./$(API_MOD)/...

## lint-web: oxlint
lint-web:
	cd $(WEB) && npm run lint

## tidy: go mod tidy (cdom + api)
tidy tidy-go:
	cd $(CDOM_MOD) && go mod tidy
	cd $(API_MOD) && go mod tidy

## codegraph: Init/refresh CodeGraph index
codegraph:
	./scripts/codegraph-init.sh

## clean: Remove bin/, .run/, web dist
clean:
	rm -rf $(BIN_DIR) .run $(WEB)/dist

# --- Docker (context = repo root; images build Go inside) ---

## docker-build-all: docforge-api + docforge-worker images
docker-build-all: docker-build-api docker-build-worker

## docker-package: Alias docker-build-all
docker-package: docker-build-all

## docker-build-api: Multi-stage API image
docker-build-api:
	docker build -f $(API_MOD)/Dockerfile \
		-t $(API_IMAGE):$(IMAGE_TAG) \
		-t $(API_IMAGE):local \
		.

## docker-build-worker: Multi-stage worker image
docker-build-worker:
	docker build -f $(API_MOD)/Dockerfile.worker \
		-t $(WORKER_IMAGE):$(IMAGE_TAG) \
		-t $(WORKER_IMAGE):local \
		.

## docker-push-all: Push API + worker (docker login first)
docker-push-all: docker-push-api docker-push-worker

docker-push-api:
	docker push $(API_IMAGE):$(IMAGE_TAG)

docker-push-worker:
	docker push $(WORKER_IMAGE):$(IMAGE_TAG)

## docker-login: Hint for GHCR login
docker-login:
	@echo "Run: docker login $(DOCKER_REGISTRY) -u USERNAME"
	@echo "Images: $(API_IMAGE):$(IMAGE_TAG)  $(WORKER_IMAGE):$(IMAGE_TAG)"
