# Implementation status

## Phase 1 — Done
Foundation API, CDOM, adapters, tests, CodeGraph, MinIO, branching model.

## Phase 2 — Processing — Done (real OCR available, stub fallback)
- PDF extract, detect, OCR interface, layout, postprocess, RabbitMQ worker
- OCR providers: HTTP Tesseract sidecar (`apps/worker`), local tesseract+pdftoppm, stub fallback

## Phase 3 — Export — Done
- Markdown + JSON + DOCX exporters, artifact APIs, table/list reconstruction
- Save + regenerate markdown from stored CDOM

## Phase 4 — Frontend — In progress
- React + Vite + TS app under `apps/web`
- Screens: Login (SSO), Dashboard, Upload, Processing (SSE + poll fallback), Document Viewer (CDOM blocks + bbox), Markdown Editor (save/regenerate), Settings (account)
- Auth: **Google + Microsoft OAuth SSO only** (no local password login)
- Local bypass via `AUTH_BYPASS=1` when IdP credentials are not configured

## Phase 5 — Hardening — In progress
- Rate limiting, request timeout (SSE excluded), security headers, CORS (PUT)
- Metrics: jobs, OCR/export duration, provider errors (`/metrics`)
- Artifact TTL cleanup sweeper
- Job retry with max attempts
- Compose overlay for OCR sidecar + Go worker

## Still open
- Heavier OCR/layout (PaddleOCR / Docling / Surya) behind the same sidecar contract
- Wire production SSO client IDs/secrets in deployment env
- Frontend e2e coverage / PDF bbox overlay highlighting
- Memory/CPU cgroup limits for worker containers
