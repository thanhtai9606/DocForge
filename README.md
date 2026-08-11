# DocForge

PDF Document Intelligence Platform — private-hosted service to upload PDFs, OCR/layout/extract into a Canonical Document Object Model (CDOM), and export Markdown, DOCX, or JSON.

## Spec & agent context

- `docs/PROJECT_SPEC.md` — architecture and technical specification (v1.1)
- `docs/CURSOR_INSTRUCTIONS.md` — coding-agent rules
- `docs/PHASE1_PLAN.md` — Phase 1 foundation tree and scope
- `.cursor/rules/` — always-on Cursor rules (architecture, CDOM, implementation, messaging)

## Messaging (MVP)

- **RabbitMQ** — job/stage queue, retries, DLQ
- **Redis** — progress cache / short-lived coordination
- **PostgreSQL** — durable metadata and job state
- **S3-compatible storage** — PDFs and artifacts

## Status

Foundation context and Cursor rules are in place. Application code starts with Phase 1 per `docs/PHASE1_PLAN.md`.
