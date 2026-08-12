# OCR providers

Replaceable OCR implementations. The orchestrator depends only on `orchestrator.OCRProvider`.
Providers must not write job/DB state.

| Provider | Package / runtime | When |
|---|---|---|
| HTTP sidecar | `apps/api/internal/providers/httocr` → `apps/worker/ocr_server.py` | `OCR_PROVIDER=http` + `OCR_URL` (or `auto` when URL is set) |
| Local Tesseract | `apps/api/internal/providers/tesseract` | `tesseract` + `pdftoppm` on PATH |
| Stub | `apps/api/internal/providers/stubocr` | fallback / tests |

Selection lives in `apps/api/internal/providers/ocrselect` (outside the providers).

Sidecar contract:

```http
POST /v1/ocr
{"document_id","page_number","language":["vi","en"],"pdf_base64"}
→ {"text","confidence","language","provider","version"}
```

CPU-only; ARM64 + x86_64 via Debian Tesseract + poppler. PaddleOCR/Docling/Surya can implement the same HTTP contract later.
