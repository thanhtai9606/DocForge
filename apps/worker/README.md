# OCR worker (Python sidecar)

Isolated CPU OCR runtime. The Go orchestrator worker consumes RabbitMQ and
calls this service; the sidecar never writes job/DB state.

```text
Go worker  --HTTP POST /v1/ocr-->  Python sidecar (pdftoppm + tesseract)
```

## Local

Needs `tesseract` (eng+vie) and `pdftoppm` (poppler-utils) on PATH:

```bash
make run-ocr
# or: python3 apps/worker/ocr_server.py
```

Health: `GET http://127.0.0.1:8090/healthz`

Then point the Go worker at it:

```bash
OCR_PROVIDER=http OCR_URL=http://127.0.0.1:8090 make run-worker
```

`OCR_PROVIDER=auto` (default) uses `OCR_URL` if set, otherwise local
`tesseract`+`pdftoppm`, otherwise the stub provider.

## Docker

```bash
make docker-build-ocr
make up-worker   # infra + ocr sidecar + Go worker
```
