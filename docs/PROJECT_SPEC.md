# PDF Document Intelligence Platform — Technical Specification

Version: 1.1
Status: Approved for implementation
Primary language: Go
AI/OCR runtime: Python where required by the AI ecosystem
Frontend: React + TypeScript
Deployment: CPU-first, ARM64 + x86_64
Messaging: RabbitMQ (job queue) + Redis (cache, progress, short-lived coordination)
Outputs: Markdown, DOCX, JSON

## 1. Product Vision

Build a private-hosted public web service that lets users upload PDF documents, analyze/extract/OCR/layout them, normalize the result into a Canonical Document Object Model (CDOM), and export Markdown, DOCX, or JSON.

The system must support:
- Digital, scanned, and mixed PDFs.
- Vietnamese and multilingual documents.
- Headings, paragraphs, lists, tables, images, code, and formulas where supported.
- Bounding boxes, reading order, language, and confidence.
- CPU-only execution.
- ARM64 and x86_64.
- Replaceable OCR/layout/export providers.
- REST API and web frontend.

The source code does not need to be public.

## 2. Architecture

```text
React Web
   |
 HTTPS / REST
   v
Go API
   |
   +--> PostgreSQL (metadata / job state)
   +--> S3-compatible storage (binaries / artifacts)
   +--> Redis (cache, progress, locks / short-lived state)
   +--> RabbitMQ (job / stage queue)
   v
Job Orchestrator (Go)
   |                    \
   v                     v
AI/OCR Worker          Export Worker
Python/native              Go
   |                         |
   +-----------+-------------+
               v
              CDOM
               |
       +-------+-------+
       |               |
    Markdown          DOCX
```

### Services

### Web Frontend
React + TypeScript. Responsible for login, dashboard, upload, processing progress, document viewer, Markdown editor, settings, and downloads.

### Go API
REST API, authentication, rate limiting, request validation, document/job/artifact metadata. Must not import OCR frameworks.

### Job Orchestrator
Creates processing plans, selects providers, publishes work to RabbitMQ, executes stages, handles retry/timeout/progress. Progress snapshots may be mirrored in Redis for fast polling/SSE.

### AI/OCR Worker
Python or native runtime where AI/OCR ecosystem requires it. Candidate providers include PaddleOCR, Docling, Marker, Surya, and future ONNX/native providers.

The worker must not own business workflow or update platform state directly. Workers consume RabbitMQ messages and report results back through the orchestrator contract.

### Export Worker
Prefer Go. Reads CDOM and exports Markdown, DOCX, JSON.

### PostgreSQL
Metadata and job state. Do not store large PDF binaries here.

### S3-compatible object storage
Original PDFs, page images, extracted assets, CDOM JSON, and generated artifacts.

### Redis
Approved for MVP. Use for:
- job/stage progress caching
- short-lived coordination / idempotency helpers
- rate-limit counters when needed
- optional SSE/polling hot path

Do not treat Redis as the source of truth for durable job state. PostgreSQL remains authoritative for documents, jobs, and artifacts.

### RabbitMQ
Approved for MVP (explicit project decision). Use for:
- job enqueue after upload/validation
- stage work distribution (OCR, layout, export, etc.)
- retry / dead-letter handling for transient worker failures

Do not introduce Kafka unless explicitly requested later.

## 3. Processing Pipeline

```text
UPLOAD
  |
VALIDATE
  |
INGEST
  |
DETECT
  |
  +--> digital --> native extraction
  |
  +--> scanned --> OCR
  |
  +--> mixed --> per-page decision
                    |
                    v
              LAYOUT ANALYSIS
                    |
                    v
                 NORMALIZE
                    |
                    v
                   CDOM
                    |
                    v
               POST-PROCESS
                    |
                    v
                  EXPORT
                    |
                    v
                ARTIFACTS
```

Stages:
`ingest`, `detect`, `extract`, `ocr`, `layout`, `normalize`, `postprocess`, `export`, `cleanup`.

## 4. CDOM

CDOM is the central contract between providers and exporters.

Root:

```json
{
  "schema_version": "1.0",
  "document_id": "uuid",
  "source": {
    "filename": "document.pdf",
    "mime_type": "application/pdf",
    "page_count": 10
  },
  "metadata": {},
  "processing": {},
  "pages": []
}
```

Page:

```json
{
  "page_number": 1,
  "width": 2480,
  "height": 3508,
  "rotation": 0,
  "language": "vi",
  "blocks": []
}
```

Block:

```json
{
  "id": "block-001",
  "type": "paragraph",
  "bbox": [100, 200, 2100, 400],
  "reading_order": 12,
  "confidence": 0.97,
  "language": "vi",
  "text": "Nội dung...",
  "children": [],
  "attributes": {}
}
```

BBox format:
`[x1, y1, x2, y2]`

Initial block types:
- heading
- paragraph
- list
- list_item
- table
- table_row
- table_cell
- image
- code
- formula
- quote
- page_break
- unknown

Tables must preserve row/cell structure. Future schema can add rowspan, colspan, cell bbox, and cell confidence.

Image blocks use an asset reference:

```json
{
  "type": "image",
  "attributes": {
    "asset_id": "asset-123",
    "alt": "Sơ đồ hệ thống"
  }
}
```

Confidence can exist at document/page/block/cell/token level. Block confidence is the minimum requirement.

Each block has `reading_order`. Layout providers are responsible for determining final reading order.

Semantic tags are supported:

```json
{
  "attributes": {
    "semantic_tags": ["invoice_number"]
  }
}
```

Processing metadata must record:
- provider_name
- provider_version
- model_name
- model_version
- cdm_schema_version

## 5. Provider SDK

Capabilities:
- OCRProvider
- LayoutProvider
- Exporter
- PostProcessor

OCR:

```go
type OCRProvider interface {
    Name() string
    Version() string
    Capabilities() Capabilities
    Process(ctx context.Context, req OCRRequest) (OCRResult, error)
}

type OCRRequest struct {
    DocumentID string
    PageNumber int
    AssetURI   string
    Language   []string
}
```

Layout:

```go
type LayoutProvider interface {
    Name() string
    Version() string
    Capabilities() Capabilities
    Analyze(ctx context.Context, req LayoutRequest) (LayoutResult, error)
}
```

Exporter:

```go
type Exporter interface {
    Name() string
    Version() string
    SupportedFormats() []string
    Export(ctx context.Context, document *Document) (Artifact, error)
}
```

Provider manifest:

```yaml
name: paddleocr
version: 1.0.0
type: ocr

capabilities:
  languages:
    - vi
    - en
  cpu: true
  arm64: true
  x86_64: true

limits:
  max_pages: 100
```

Provider selection considers document type, language, capability, architecture, CPU/GPU availability, health, and configured priority.

Providers must be isolated and must not directly update DB/job state or bypass the orchestrator.

## 6. REST API

Base path:

`/api/v1`

Create document:

```http
POST /api/v1/documents
Content-Type: multipart/form-data
```

Response:

```json
{
  "document_id": "uuid",
  "job_id": "uuid",
  "status": "queued"
}
```

Get job:

```http
GET /api/v1/jobs/{job_id}
```

Response:

```json
{
  "job_id": "uuid",
  "document_id": "uuid",
  "status": "processing",
  "stage": "ocr",
  "progress": 42,
  "error": null
}
```

Get document:

```http
GET /api/v1/documents/{document_id}
```

Get artifacts:

```http
GET /api/v1/documents/{document_id}/artifacts
```

Download artifact:

```http
GET /api/v1/artifacts/{artifact_id}/download
```

Cancel:

```http
POST /api/v1/jobs/{job_id}/cancel
```

Job states:
- queued
- processing
- completed
- failed
- cancelled

Error response:

```json
{
  "error": {
    "code": "INVALID_PDF",
    "message": "The uploaded file is not a valid PDF.",
    "retryable": false,
    "request_id": "uuid"
  }
}
```

Initial error codes:
`INVALID_PDF`, `UNSUPPORTED_DOCUMENT`, `FILE_TOO_LARGE`, `PAGE_LIMIT_EXCEEDED`, `OCR_TIMEOUT`, `OCR_PROVIDER_ERROR`, `LAYOUT_FAILED`, `NORMALIZATION_FAILED`, `EXPORT_FAILED`, `STORAGE_ERROR`, `JOB_CANCELLED`.

## 7. Export

Markdown mapping:
- heading -> `#`
- paragraph -> text
- list -> Markdown list
- table -> Markdown table
- image -> asset reference
- code -> fenced code block
- quote -> `>`
- formula -> LaTeX when available
- page_break -> separator

DOCX mapping:
- heading -> Word heading styles
- paragraph -> Paragraph
- list -> list styles
- table -> Word Table
- image -> inline image
- code -> code style

Exporters only consume CDOM.

## 8. Frontend

Technology:
- React
- TypeScript
- Vite
- React Router
- TanStack Query

Do not use micro-frontends.

Suggested structure:

```text
apps/web/
  src/
    app/
      router/
      providers/
    features/
      auth/
      dashboard/
      upload/
      processing/
      documents/
      editor/
      settings/
    components/
      ui/
      layout/
    services/
      api/
      upload/
    hooks/
    types/
    utils/
```

Screens:
- Login
- Dashboard
- Upload
- Processing
- Document Viewer
- Markdown Editor
- Settings

### Login
Components: LoginPage, LoginForm, EmailInput, PasswordInput, SubmitButton, ErrorMessage.

### Dashboard
Show recent documents, processing jobs, completed jobs, failed jobs, and upload action.

Document row:
filename, status, created_at, page_count, output formats, actions.

Actions:
View, Download, Delete, Retry.

### Upload
Support drag/drop, file picker, validation, size limit, output selection, and upload progress.

### Processing
Show job status, current stage, progress, elapsed time, page progress, errors, and cancellation.

Use SSE for progress when available; polling is acceptable as fallback. Redis-backed progress is preferred for the hot path.

### Document Viewer
Two-pane layout:
- PDF preview
- document structure/CDOM view

CDOM bbox/page/block IDs allow future PDF highlighting.

### Markdown Editor
Split source/editor and rendered preview.

Actions:
Save, Download, Copy, Regenerate.

### Settings
Default output, language, theme, processing preferences, account.

Provider configuration is not exposed to normal users.

Frontend API calls must go through an API client layer. Do not scatter raw fetch calls throughout features.

Use:
- TanStack Query for server state.
- Local state for forms/UI.
- Auth provider for authentication.
- Global state only when necessary.

## 9. Repository

```text
project/
├── apps/
│   ├── api/
│   ├── worker/
│   └── web/
├── packages/
│   └── cdom/
├── providers/
│   ├── ocr/
│   ├── layout/
│   └── exporters/
├── docs/
├── deployments/
├── tests/
│   └── fixtures/
├── scripts/
└── README.md
```

Go API:

```text
apps/api/
  cmd/
  internal/
    api/
    application/
    domain/
    infrastructure/
    storage/
    jobs/
```

Domain must not import infrastructure.

## 10. Security

PDF is untrusted input.

Required:
- file size limit
- page count limit
- MIME/content validation
- parser/worker isolation
- filename sanitization
- rate limiting
- timeout
- memory/CPU limits
- temporary file cleanup
- artifact expiration
- Markdown/HTML sanitization

Never render untrusted raw HTML from Markdown.

## 11. Retry and Idempotency

Retry transient failures:
- worker unavailable
- timeout
- temporary storage failure
- temporary network failure
- RabbitMQ redelivery / nack for transient worker failures

Do not retry invalid input/configuration errors.

Default:
- max_attempts = 3
- exponential backoff

Stage identity:
`job_id + document_id + stage + attempt`

Before rerunning a stage, check whether its expected artifact already exists.

Use RabbitMQ dead-letter queues for exhausted retries. Persist final failure state in PostgreSQL.

## 12. Observability

Track:
- request_id
- job_id
- document_id
- stage
- provider
- provider_version
- duration
- page_count
- error

Metrics:
- jobs_total
- jobs_success
- jobs_failed
- processing_duration
- ocr_duration
- export_duration
- pages_processed
- provider_errors

## 13. Testing

Backend:
- unit
- integration
- API
- CDOM validation
- pipeline
- provider contract
- RabbitMQ publish/consume contract
- Redis progress cache behavior

Frontend:
- component
- API mocking
- user-flow
- upload
- editor

AI fixture PDFs:
- simple-text.pdf
- vietnamese.pdf
- scanned.pdf
- tables.pdf
- mixed-layout.pdf
- multi-column.pdf

Provider contract must test startup, health, process, timeout, errors, CDOM conversion, and confidence validity.

## 14. MVP Implementation Order

### Phase 1 — Foundation
- Go project
- configuration
- logging
- PostgreSQL
- object storage
- Redis
- RabbitMQ wiring (connection, declare queues/exchanges)
- CDOM
- document/job domain
- REST API

### Phase 2 — Processing
- orchestrator
- PDF validation
- document detection
- native extraction
- AI worker interface
- OCR provider
- RabbitMQ stage consumers

### Phase 3 — Export
- Markdown exporter
- DOCX exporter
- JSON exporter
- artifact management

### Phase 4 — Frontend
- Login
- Dashboard
- Upload
- Processing
- Document Viewer
- Markdown Editor
- Download

### Phase 5 — Hardening
- rate limiting
- resource limits
- retry
- timeout
- observability
- security
- cleanup

## 15. Explicitly Out of MVP

Do not add unless explicitly requested:
- Kafka
- Kubernetes
- micro-frontends
- GPU requirement
- LLM in every processing stage
- complex distributed tracing infrastructure

Note: RabbitMQ and Redis are **in scope** for MVP by explicit project decision.

Architecture must allow later extensions without changing CDOM/domain/API contracts.

## 16. Future Extensions

Potential future capabilities:
- invoice extraction
- contract extraction
- CV parsing
- semantic search
- document Q&A
- summarization
- classification
- RAG
- document comparison
- HTML
- EPUB
- LaTeX

These must not complicate the initial PDF -> Markdown/DOCX MVP.

## 17. Definition of Done

MVP is complete when:
- PDF upload works.
- Job creation works.
- Job is queued via RabbitMQ.
- Job progress works (PostgreSQL authoritative; Redis optional hot path).
- Digital PDFs can be extracted.
- Scanned PDFs can be OCRed.
- Layout is detected.
- Result is normalized to CDOM.
- Markdown export works.
- DOCX export works.
- Artifacts can be downloaded.
- Frontend displays results.
- Retry/error handling exists.
- Resource limits exist.
- CPU-only operation works.
- ARM64 works.
- x86_64 works.
- NVIDIA GPU is not required.

## 18. Architecture Invariant

The most important boundary is:

```text
             PROVIDERS
                 |
                 v
      +---------------------+
      |        CDOM         |
      | Canonical Document  |
      |       Model         |
      +----------+----------+
                 |
                 v
              EXPORTERS
```

OCR engine, layout engine, model, and runtime may change.

These must remain stable:
- CDOM
- API contracts
- Domain contracts

This prevents lock-in to any specific OCR framework or model.

## 19. AI Coding Agent Rules

When using Cursor/Claude Code/other coding agents:

1. Read this specification before writing code.
2. Do not change architecture without explicit approval.
3. Do not bypass CDOM.
4. Do not let API depend on OCR implementation.
5. Do not let exporters depend on OCR implementation.
6. Keep handlers thin.
7. Keep business logic out of infrastructure.
8. Use dependency injection.
9. Use small interfaces.
10. Add tests for new behavior.
11. Do not hard-code configuration.
12. Keep providers isolated.
13. Do not introduce Kubernetes for MVP.
14. RabbitMQ and Redis are approved messaging/cache infrastructure; do not introduce Kafka unless requested.
15. Do not require NVIDIA GPU.
16. Prefer simple implementations over speculative abstractions.
17. Preserve ARM64 and x86_64 compatibility.
18. Keep Python isolated to AI/OCR responsibilities unless a native Go implementation is deliberately chosen.
19. Do not add a new external dependency when the standard library or an existing project dependency is sufficient.
20. Before implementing a major component, verify that it conforms to the CDOM and service boundaries in this document.
21. PostgreSQL is the durable source of truth for documents/jobs/artifacts; Redis is cache/coordination only; RabbitMQ is the work queue.
