# DocForge

PDF Document Intelligence Platform — private-hosted service to upload PDFs, OCR/layout/extract into a Canonical Document Object Model (CDOM), and export Markdown, DOCX, or JSON.

## Spec & agent context

- `docs/PROJECT_SPEC.md` — architecture and technical specification (v1.1)
- `docs/CURSOR_INSTRUCTIONS.md` — coding-agent rules
- `docs/PHASE1_PLAN.md` — Phase 1 foundation tree and scope
- `.cursor/rules/` — always-on Cursor rules (architecture, CDOM, implementation, messaging)

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
