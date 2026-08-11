# Implementation status

## Phase 1 — Done
Foundation API, CDOM, adapters, tests, CodeGraph, MinIO, branching model.

## Phase 2/3 — Production pipeline (current)
- PDF extract via `rsc.io/pdf` with positioned text + bounded page concurrency
- Document classify digital/scanned/mixed
- Parallel OCR fan-out for scanned/mixed pages (replaceable provider; stub default)
- Geometric layout provider assigns final `reading_order`
- CDOM normalize + artifact persistence
- Exporters: Markdown + JSON (CDOM-only)
- Idempotent artifact reuse before re-running expensive stages
- RabbitMQ worker consumer with permanent vs retryable failure handling

## Still open
- Real OCR runtime (PaddleOCR/Python or ONNX)
- Stronger table/list reconstruction & DOCX exporter
- React frontend
- Hardening: quotas, cleanup TTL, richer metrics
