# Implementation status

## Phase 1 — Done
Foundation API, CDOM, adapters, tests, CodeGraph, MinIO, branching model.

## Phase 2 — Processing — Done (OCR provider still stubbed)
- PDF extract via `rsc.io/pdf` with positioned text + bounded page concurrency
- Large horizontal gaps split into column segments for table layout
- Document classify digital/scanned/mixed
- Parallel OCR fan-out for scanned/mixed pages (replaceable provider; stub default)
- Geometric layout provider assigns final `reading_order`
- List/table reconstruction: bullets/numbered, pipe tables, multi-space columns, geometric column clusters
- CDOM postprocess cleanup before normalize
- RabbitMQ worker consumer with permanent vs retryable failure handling

## Phase 3 — Export — Done
- Exporters: Markdown + JSON + DOCX (CDOM-only)
- Export selection respects document `output_formats`
- Idempotent artifact reuse before re-running expensive stages
- Artifact management: metadata GET, list filters (`kind`/`format`), friendly download filenames, Content-Length

## Still open
- Real OCR runtime (PaddleOCR/Python or ONNX)
- React frontend (Phase 4)
- Hardening: quotas, cleanup TTL, richer metrics (Phase 5)
