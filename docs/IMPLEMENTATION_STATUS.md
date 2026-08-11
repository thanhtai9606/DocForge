# Implementation status

## Phase 1 — Done
Foundation API, CDOM, adapters, tests, CodeGraph, MinIO, branching model.

## Phase 2 — Processing — Done (OCR provider still stubbed)
- PDF extract via `rsc.io/pdf` with positioned text + bounded page concurrency
- Document classify digital/scanned/mixed
- Parallel OCR fan-out for scanned/mixed pages (replaceable provider; stub default)
- Geometric layout provider assigns final `reading_order`
- List/table reconstruction heuristics (bullets/numbered + pipe tables)
- CDOM normalize + artifact persistence
- RabbitMQ worker consumer with permanent vs retryable failure handling

## Phase 3 — Export — Done
- Exporters: Markdown + JSON + DOCX (CDOM-only)
- Export selection respects document `output_formats`
- Idempotent artifact reuse before re-running expensive stages

## Still open
- Real OCR runtime (PaddleOCR/Python or ONNX)
- Stronger geometric table reconstruction (column clustering beyond pipes)
- React frontend
- Hardening: quotas, cleanup TTL, richer metrics
