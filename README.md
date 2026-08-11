# DocForge

PDF Document Intelligence Platform — private-hosted service to upload PDFs, OCR/layout/extract into a Canonical Document Object Model (CDOM), and export Markdown, DOCX, or JSON.

## Spec & agent context

- `docs/PROJECT_SPEC.md` — architecture and technical specification (v1.1)
- `docs/CURSOR_INSTRUCTIONS.md` — coding-agent rules
- `docs/PHASE1_PLAN.md` — Phase 1 foundation tree and scope
- `docs/CODEGRAPH.md` — CodeGraph setup (fewer AI discovery tokens)
- `AGENTS.md` — agent notes including CodeGraph CLI fallback
- `.cursor/rules/` — always-on Cursor rules (architecture, CDOM, implementation, messaging, codegraph)
- `.cursor/mcp.json` — CodeGraph MCP server for Cursor

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
