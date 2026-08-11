# Worker

Phase 2 processing worker lives in the API Go module:

```bash
go run ./apps/api/cmd/worker
```

It consumes RabbitMQ `process` jobs and runs the orchestrator pipeline:

`detect → extract|ocr → normalize (CDOM)`

OCR uses a replaceable provider interface; default is the CPU stub (`internal/providers/stubocr`).
