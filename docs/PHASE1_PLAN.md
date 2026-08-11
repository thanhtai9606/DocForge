# Phase 1 — Foundation Plan

Status: Approved direction (architecture unchanged except RabbitMQ + Redis in MVP)

## Target repository tree

```text
project/
├── apps/
│   ├── api/
│   │   ├── cmd/api/main.go
│   │   ├── internal/
│   │   │   ├── api/
│   │   │   ├── application/
│   │   │   ├── domain/
│   │   │   ├── infrastructure/
│   │   │   │   ├── postgres/
│   │   │   │   ├── redis/
│   │   │   │   ├── rabbitmq/
│   │   │   │   └── s3/
│   │   │   ├── storage/
│   │   │   └── jobs/
│   │   ├── configs/
│   │   ├── migrations/
│   │   └── go.mod
│   ├── worker/                   # stub in Phase 1
│   └── web/                      # stub in Phase 1
├── packages/
│   └── cdom/
├── providers/
│   ├── ocr/
│   ├── layout/
│   └── exporters/
├── docs/
│   ├── PROJECT_SPEC.md
│   ├── CURSOR_INSTRUCTIONS.md
│   └── PHASE1_PLAN.md
├── deployments/
│   └── docker-compose.yml        # postgres + minio + redis + rabbitmq
├── tests/
│   └── fixtures/
├── scripts/
└── README.md
```

## Phase 1 scope

1. Docs & scaffold matching the tree above.
2. CDOM Go package (`packages/cdom`) with validation tests.
3. Domain models: Document, Job, Artifact; job states; API error codes.
4. Config + structured logging (`request_id`).
5. PostgreSQL migrations + repositories (interfaces; domain does not import infra).
6. S3-compatible object storage abstraction (MinIO locally).
7. Redis client wiring for progress/cache helpers.
8. RabbitMQ connection + declare exchanges/queues (publish path for queued jobs).
9. REST API skeleton under `/api/v1`:
   - `POST /documents`
   - `GET /jobs/{id}`
   - `GET /documents/{id}`
   - `GET /documents/{id}/artifacts`
   - `GET /artifacts/{id}/download`
   - `POST /jobs/{id}/cancel`
10. Upload creates durable DB records, stores PDF in object storage, enqueues job on RabbitMQ, mirrors progress hint in Redis when useful.
11. Tests for CDOM, domain transitions, API happy-path.

## Explicitly not in Phase 1

- Full orchestrator pipeline stages
- OCR/layout providers
- Markdown/DOCX export
- React frontend screens
- Full hardening (rate limits, cleanup policies) — later phase

## Assumptions

- Go module path: `github.com/thanhtai9606/DocForge`
- Auth UI is Phase 4; Phase 1 may stub/dev-auth or leave auth open behind config
- PostgreSQL remains authoritative; Redis is never durable SoT; RabbitMQ carries work messages
