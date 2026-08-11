# Cursor Implementation Instructions

Read `docs/PROJECT_SPEC.md` completely before implementing the project.
Also follow `.cursor/rules/` for always-on project constraints.

## Messaging decision (approved)

- **RabbitMQ**: job/stage work queue, retries, dead-letter handling.
- **Redis**: progress cache, short-lived coordination, rate-limit helpers.
- **PostgreSQL**: durable source of truth for documents, jobs, artifacts.
- **Do not** introduce Kafka unless explicitly requested.

Implementation order:
1. Foundation/domain/CDOM
2. Go API
3. Storage (Postgres + MinIO) + Redis + RabbitMQ wiring
4. Job orchestration
5. AI worker/provider contract
6. Markdown/DOCX exporters
7. React frontend
8. Tests and hardening

Do not redesign the architecture during implementation.
Do not couple API/exporters to a concrete OCR provider.
Do not require GPU.
Keep ARM64 and x86_64 compatibility.
Ask for explicit approval before introducing Kafka, Kubernetes, micro-frontends, or a mandatory LLM stage.

For every implementation task:
- identify the relevant section of `docs/PROJECT_SPEC.md`;
- implement the smallest coherent change;
- add/update tests;
- keep interfaces stable;
- report assumptions and deviations.
