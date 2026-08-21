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
ENV_FILE ?= apps/api/configs/local.env
ENV_EXAMPLE := apps/api/configs/local.env.example

# Optional docker registry/tag overrides from ENV_FILE (make args still win)
_read_env = $(shell if [ -f $(ENV_FILE) ]; then grep -E '^$(1)=' $(ENV_FILE) 2>/dev/null | head -1 | cut -d= -f2- | tr -d '\r"' | sed 's/^[[:space:]]*//;s/[[:space:]]*$$//'; fi)
ENV_DOCKER_REGISTRY := $(call _read_env,DOCKER_REGISTRY)
ENV_DOCKER_NAMESPACE := $(call _read_env,DOCKER_NAMESPACE)
ENV_IMAGE_TAG := $(call _read_env,IMAGE_TAG)
ENV_VERSION := $(call _read_env,VERSION)

DOCKER_REGISTRY ?= $(if $(strip $(ENV_DOCKER_REGISTRY)),$(ENV_DOCKER_REGISTRY),docker.io)
DOCKER_NAMESPACE ?= $(if $(strip $(ENV_DOCKER_NAMESPACE)),$(ENV_DOCKER_NAMESPACE),becamexidc2020)
GHCR_OWNER ?= $(DOCKER_NAMESPACE)
DOCKER_IMAGE_PREFIX := $(DOCKER_REGISTRY)/$(DOCKER_NAMESPACE)
GIT_SHA := $(shell git rev-parse --short HEAD 2>/dev/null | tr '/:' '--')
ifeq ($(strip $(GIT_SHA)),)
GIT_SHA := latest
endif
VERSION ?= $(ENV_VERSION)
IMAGE_TAG ?= $(if $(strip $(ENV_IMAGE_TAG)),$(ENV_IMAGE_TAG),$(if $(strip $(VERSION)),$(VERSION),$(GIT_SHA)))

API_IMAGE := $(DOCKER_IMAGE_PREFIX)/docforge-api
WORKER_IMAGE := $(DOCKER_IMAGE_PREFIX)/docforge-worker
OCR_IMAGE := $(DOCKER_IMAGE_PREFIX)/docforge-ocr
WEB_IMAGE := $(DOCKER_IMAGE_PREFIX)/docforge-web

COMPOSE_INFRA := docker compose -f deployments/docker-compose.yml
COMPOSE_API := docker compose -f deployments/docker-compose.yml -f deployments/docker-compose.api.yml
COMPOSE_WORKER := docker compose -f deployments/docker-compose.yml -f deployments/docker-compose.worker.yml

.PHONY: help \
	api worker build build-go build-api build-worker build-linux-go build-linux-all build-all \
	build-web build-frontend \
	run-api run-worker run-api-memory run-web run-ocr run-all \
	infra infra-down infra-logs up down up-api down-api up-worker down-worker \
	test test-go test-cdom test-api test-web fmt vet lint-web tidy tidy-go clean env codegraph \
	docker-build-all docker-package docker-print docker-push-all login docker-login \
	docker-build-api docker-build-worker docker-build-ocr docker-build-web \
	docker-push-api docker-push-worker docker-push-ocr docker-push-web

## help: Show Makefile targets
help:
	@echo "DocForge — Makefile targets (run from repo root):"
	@echo ""
	@grep -hE '^## ' $(MAKEFILE_LIST) | sed 's/^## /  make /' | column -t -s ':' 2>/dev/null || grep -hE '^## ' $(MAKEFILE_LIST) | sed 's/^## /  make /'
	@echo ""
	@echo "  DOCKER_REGISTRY=$(DOCKER_REGISTRY)  DOCKER_NAMESPACE=$(DOCKER_NAMESPACE)  IMAGE_TAG=$(IMAGE_TAG)  VERSION=$(VERSION)  GIT_SHA=$(GIT_SHA)"

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

## run-ocr: Python Tesseract sidecar on port 8090 (needs tesseract + pdftoppm)
run-ocr:
	python3 apps/worker/ocr_server.py

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

## up-api: Infra + API container (published or locally tagged image)
up-api:
	DOCKER_REGISTRY=$(DOCKER_REGISTRY) DOCKER_NAMESPACE=$(DOCKER_NAMESPACE) API_IMAGE_TAG=$(IMAGE_TAG) $(COMPOSE_API) up -d

## down-api: Stop infra + API overlay
down-api:
	DOCKER_REGISTRY=$(DOCKER_REGISTRY) DOCKER_NAMESPACE=$(DOCKER_NAMESPACE) API_IMAGE_TAG=$(IMAGE_TAG) $(COMPOSE_API) down

## up-worker: Infra + OCR sidecar + Go worker
up-worker:
	DOCKER_REGISTRY=$(DOCKER_REGISTRY) DOCKER_NAMESPACE=$(DOCKER_NAMESPACE) WORKER_IMAGE_TAG=$(IMAGE_TAG) OCR_IMAGE=$(OCR_IMAGE):$(IMAGE_TAG) $(COMPOSE_WORKER) up -d --build

## down-worker: Stop infra + worker overlay
down-worker:
	DOCKER_REGISTRY=$(DOCKER_REGISTRY) DOCKER_NAMESPACE=$(DOCKER_NAMESPACE) WORKER_IMAGE_TAG=$(IMAGE_TAG) $(COMPOSE_WORKER) down

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

define _docker_build_tags
-t $(1):$(IMAGE_TAG) \
-t $(1):latest \
-t $(1):local \
$(if $(filter-out $(IMAGE_TAG),$(GIT_SHA)),-t $(1):$(GIT_SHA),) \
$(if $(and $(strip $(VERSION)),$(filter-out $(IMAGE_TAG),$(VERSION))),-t $(1):$(VERSION),)
endef

define _docker_push_tags
	@set -e; \
	img="$(1)"; \
	echo "Push $$img:$(IMAGE_TAG)"; docker push "$$img:$(IMAGE_TAG)"; \
	echo "Push $$img:latest"; docker push "$$img:latest"; \
	if [ "$(IMAGE_TAG)" != "$(GIT_SHA)" ]; then echo "Push $$img:$(GIT_SHA)"; docker push "$$img:$(GIT_SHA)"; fi; \
	if [ -n "$(strip $(VERSION))" ] && [ "$(IMAGE_TAG)" != "$(VERSION)" ]; then echo "Push $$img:$(VERSION)"; docker push "$$img:$(VERSION)"; fi
endef

## docker-print: Show registry + tags that build/push will use
docker-print:
	@echo "Registry : $(DOCKER_IMAGE_PREFIX)"
	@echo "IMAGE_TAG: $(IMAGE_TAG)  latest  local  $(GIT_SHA)$(if $(strip $(VERSION)),  $(VERSION),)"
	@echo "Example  : $(API_IMAGE):$(IMAGE_TAG)  $(WEB_IMAGE):$(IMAGE_TAG)"

## docker-build-all: API + worker + OCR + web dashboard images
docker-build-all: docker-print docker-build-api docker-build-worker docker-build-ocr docker-build-web

## docker-package: Alias docker-build-all
docker-package: docker-build-all

## docker-build-api: Multi-stage API image
docker-build-api:
	docker build -f $(API_MOD)/Dockerfile \
		$(call _docker_build_tags,$(API_IMAGE)) \
		.

## docker-build-worker: Multi-stage worker image
docker-build-worker:
	docker build -f $(API_MOD)/Dockerfile.worker \
		$(call _docker_build_tags,$(WORKER_IMAGE)) \
		.

## docker-build-ocr: Tesseract CPU OCR sidecar
docker-build-ocr:
	docker build -f apps/worker/Dockerfile \
		$(call _docker_build_tags,$(OCR_IMAGE)) \
		-t docforge-ocr:local \
		apps/worker

## docker-build-web: React dashboard (nginx + proxy /api → API service)
docker-build-web:
	docker build -f $(WEB)/Dockerfile \
		$(call _docker_build_tags,$(WEB_IMAGE)) \
		$(WEB)

## docker-push-all: Push API + worker + OCR + web to Docker Hub (run make login first)
docker-push-all: docker-push-api docker-push-worker docker-push-ocr docker-push-web

docker-push-api:
	$(call _docker_push_tags,$(API_IMAGE))

docker-push-worker:
	$(call _docker_push_tags,$(WORKER_IMAGE))

docker-push-ocr:
	$(call _docker_push_tags,$(OCR_IMAGE))

docker-push-web:
	$(call _docker_push_tags,$(WEB_IMAGE))

## login: Log in to container registry (alias docker-login)
login: docker-login

## docker-login: Log in to Docker Hub (default) or GHCR when DOCKER_REGISTRY=ghcr.io
docker-login:
	@if [ "$(DOCKER_REGISTRY)" = "ghcr.io" ]; then \
		if command -v gh >/dev/null 2>&1 && gh auth status >/dev/null 2>&1; then \
			echo "Logging in to $(DOCKER_REGISTRY) as $(DOCKER_NAMESPACE) via gh..."; \
			gh auth token | docker login $(DOCKER_REGISTRY) -u $(DOCKER_NAMESPACE) --password-stdin; \
		elif [ -n "$$GITHUB_TOKEN" ]; then \
			echo "Logging in to $(DOCKER_REGISTRY) as $(DOCKER_NAMESPACE) via GITHUB_TOKEN..."; \
			printf '%s\n' "$$GITHUB_TOKEN" | docker login $(DOCKER_REGISTRY) -u $(DOCKER_NAMESPACE) --password-stdin; \
		else \
			echo "GHCR login failed: need gh CLI or GITHUB_TOKEN."; \
			echo "  gh auth login && make login"; \
			echo "  GITHUB_TOKEN=<pat> make login"; \
			exit 1; \
		fi; \
	elif [ -n "$$DOCKERHUB_TOKEN" ] || [ -n "$$DOCKER_PASSWORD" ]; then \
		echo "Logging in to $(DOCKER_REGISTRY) as $(DOCKER_NAMESPACE)..."; \
		printf '%s\n' "$${DOCKERHUB_TOKEN:-$$DOCKER_PASSWORD}" | docker login $(DOCKER_REGISTRY) -u $(DOCKER_NAMESPACE) --password-stdin; \
	else \
		echo "Docker Hub login required before push."; \
		echo ""; \
		echo "Option A (recommended):"; \
		echo "  DOCKERHUB_TOKEN=<access-token> make login"; \
		echo ""; \
		echo "Option B (interactive):"; \
		echo "  docker login $(DOCKER_REGISTRY) -u $(DOCKER_NAMESPACE)"; \
		echo ""; \
		echo "Push example: $(API_IMAGE):$(IMAGE_TAG)"; \
		exit 1; \
	fi
