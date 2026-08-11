# Implementation status

## Phase 1 — Done
- Monorepo scaffold (`apps/api`, `packages/cdom`, providers stubs, compose)
- CDOM types + validation
- Domain models (Document/Job/Artifact), job transitions, AppError codes
- Config + logging + REST `/api/v1`
- Adapters: Postgres, Redis, RabbitMQ, MinIO, in-memory doubles
- Unit/API tests

## Phase 2 — In progress (this PR)
- Job orchestrator (`detect → extract|ocr → normalize`)
- RabbitMQ worker consumer (`apps/api/cmd/worker`)
- Digital vs scanned detection + heuristic native extract
- Replaceable OCR provider interface + CPU stub provider
- CDOM artifact written to object storage; job progress mirrored in Redis

## Not done yet
- Real OCR providers (PaddleOCR/Python, etc.)
- Layout analysis provider
- Markdown/DOCX exporters (Phase 3)
- React frontend (Phase 4)
- Full hardening (Phase 5)
