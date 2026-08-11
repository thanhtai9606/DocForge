# Implementation status (Phase 1)

## Done
- Monorepo scaffold (`apps/api`, `packages/cdom`, providers stubs, compose)
- CDOM types + validation
- Domain models (Document/Job/Artifact), job transitions, AppError codes
- Config + structured logging (`request_id`)
- Application use cases: upload, get job/document, artifacts, cancel
- REST `/api/v1` handlers + `/healthz`
- Adapters: Postgres, Redis progress, RabbitMQ publisher (+ DLQ declare), object store, in-memory test doubles
- Unit/API tests for CDOM, domain, config, logging, application, API, memory adapters, RabbitMQ message shape

## Not done yet (later phases)
- Orchestrator pipeline stages (detect/extract/ocr/layout/normalize)
- AI/OCR worker consumers
- Markdown/DOCX/JSON exporters
- React frontend
- Full hardening (rate limits, cleanup policies, observability dashboards)
