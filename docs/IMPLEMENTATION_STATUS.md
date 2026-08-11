# Implementation status

## Phase 1 — Done
Foundation API, CDOM, adapters, tests, CodeGraph, MinIO, branching model.

## Phase 2 — Processing — Done (OCR provider still stubbed)
- PDF extract, detect, OCR interface (stub), layout, postprocess, RabbitMQ worker

## Phase 3 — Export — Done
- Markdown + JSON + DOCX exporters, artifact APIs, table/list reconstruction

## Phase 4 — Frontend — In progress
- React + Vite + TS app under `apps/web`
- Screens: Login (SSO), Dashboard, Upload, Processing, Document Viewer, Markdown Editor, Settings
- Auth: **Google + Microsoft OAuth SSO only** (no local password login)
- Local bypass via `AUTH_BYPASS=1` when IdP credentials are not configured

## Phase 5 — Hardening — In progress
- Rate limiting, request timeout, security headers, CORS
- Metrics endpoint (`/metrics`)
- Artifact TTL cleanup sweeper
- Job retry with max attempts

## Still open
- Real OCR runtime (PaddleOCR/Python or ONNX)
- Wire production SSO client IDs/secrets in deployment env
- Frontend polish / additional e2e coverage
