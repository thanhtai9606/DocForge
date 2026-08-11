# OCR providers

Replaceable OCR implementations. The orchestrator depends only on `orchestrator.OCRProvider`.

- Default Phase 2 wiring: `apps/api/internal/providers/stubocr` (CPU stub)
- Future: PaddleOCR / Docling / Surya adapters can live here as isolated packages and must not update job/DB state directly.
