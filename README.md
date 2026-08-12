# DocForge

PDF Document Intelligence Platform — private-hosted service to upload PDFs, OCR/layout/extract into a Canonical Document Object Model (CDOM), and export Markdown, DOCX, or JSON.

## Spec & agent context

- `docs/PROJECT_SPEC.md` — architecture and technical specification (v1.1)
- `docs/CURSOR_INSTRUCTIONS.md` — coding-agent rules
- `docs/PHASE1_PLAN.md` — Phase 1 foundation tree and scope
- `docs/CODEGRAPH.md` — CodeGraph setup (fewer AI discovery tokens)
- `docs/BRANCHING.md` — `develop` integration / `main` release flow
- `AGENTS.md` — agent notes including CodeGraph CLI fallback
- `.cursor/rules/` — always-on Cursor rules (architecture, CDOM, implementation, messaging, codegraph, branching)
- `.cursor/mcp.json` — CodeGraph MCP server for Cursor

## Branching

- **`develop`** — integration branch; open all feature PRs here first
- **`main`** — release branch; GitHub Actions image publish runs only here

See `docs/BRANCHING.md`.

## CodeGraph (AI token optimization)

Index once per clone, then agents query the graph instead of crawling files:

```bash
./scripts/codegraph-init.sh
# or: npx -y @colbymchenry/codegraph init
```

Restart Cursor after clone so MCP loads. See `docs/CODEGRAPH.md`.

## Messaging & storage (MVP)

- **RabbitMQ** — job/stage queue, retries, DLQ
- **Redis** — progress cache / short-lived coordination
- **PostgreSQL** — durable metadata and job state
- **MinIO** — PDFs and artifacts (self-hosted object storage)

## Makefile

```bash
make help              # all targets
make env               # copy apps/api/configs/local.env.example → local.env
make infra             # postgres + redis + rabbitmq + minio
make build-go          # bin/api + bin/worker
make run-api           # HTTP API on port 8080
make run-worker        # RabbitMQ processing worker
make run-web           # Vite frontend (proxies /api → port 8080)
make run-all           # infra + API + worker (one terminal)
make run-ocr           # Tesseract CPU sidecar :8090
make test              # CDOM + API tests
make docker-build-all  # api + worker + ocr images
```

### Run dependencies

```bash
make infra
# or: docker compose -f deployments/docker-compose.yml up -d
```

### Run API

```bash
make env
make run-api
```

For local smoke without infra:

```bash
make run-api-memory
```

### Run worker (Phase 2)

Requires compose infra (Postgres, Redis, RabbitMQ, MinIO):

```bash
make run-worker
# optional real OCR: make run-ocr && OCR_PROVIDER=http OCR_URL=http://127.0.0.1:8090 make run-worker
```

### Tests

```bash
make test
```

## Container images (GHCR)

On push to **`main`** (and `v*` tags / manual dispatch), GitHub Actions builds multi-arch images:

- `ghcr.io/<owner>/docforge-api`
- `ghcr.io/<owner>/docforge-worker`
- local OCR sidecar: `docforge-ocr` (`make docker-build-ocr`)

Platforms: `linux/amd64`, `linux/arm64`.

Tags (example):

```text
ghcr.io/<owner>/docforge-api:latest
ghcr.io/<owner>/docforge-api:sha-<commit>
ghcr.io/<owner>/docforge-worker:latest
ghcr.io/<owner>/docforge-worker:sha-<commit>
```

Local build (from repo root):

```bash
make docker-build-all
# or:
# docker build -f apps/api/Dockerfile -t docforge-api:local .
# docker build -f apps/api/Dockerfile.worker -t docforge-worker:local .
```

Run published API with local infra:

```bash
make up-api
# or:
# docker compose -f deployments/docker-compose.yml up -d
# docker compose -f deployments/docker-compose.yml -f deployments/docker-compose.api.yml up -d api
```

Package visibility: ensure the `docforge-api` GHCR package allows the intended audience (private/public) in GitHub Packages settings.
