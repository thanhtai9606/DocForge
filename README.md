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

## Phase 1 status

Foundation is implemented:

- `packages/cdom` — CDOM types + validation
- `apps/api` — domain, config, logging, REST `/api/v1`, application services
- Infrastructure adapters: Postgres, Redis, RabbitMQ, MinIO (+ in-memory for tests)
- `deployments/docker-compose.yml` — postgres, redis, rabbitmq, minio

### Run dependencies

```bash
docker compose -f deployments/docker-compose.yml up -d
```

### Run API

```bash
set -a && source apps/api/configs/local.env.example && set +a
go run ./apps/api/cmd/api
```

For local smoke without infra:

```bash
DOCFORGE_USE_MEMORY=1 go run ./apps/api/cmd/api
```

### Tests

```bash
./scripts/test.sh
```

## Container images (GHCR)

GitHub Actions builds multi-arch (`linux/amd64`, `linux/arm64`) images and pushes to GitHub Container Registry on pushes to `main` and version tags (`v*`).

Image:

```text
ghcr.io/<owner>/docforge-api:latest
ghcr.io/<owner>/docforge-api:sha-<commit>
```

Local build:

```bash
docker build -f apps/api/Dockerfile -t docforge-api:local .
```

Run published API with local infra:

```bash
docker compose -f deployments/docker-compose.yml up -d
docker compose -f deployments/docker-compose.yml -f deployments/docker-compose.api.yml up -d api
```

Package visibility: ensure the `docforge-api` GHCR package allows the intended audience (private/public) in GitHub Packages settings.
