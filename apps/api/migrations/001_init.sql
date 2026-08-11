-- +goose Up
CREATE TABLE IF NOT EXISTS documents (
    id UUID PRIMARY KEY,
    filename TEXT NOT NULL,
    content_type TEXT NOT NULL,
    size_bytes BIGINT NOT NULL,
    page_count INT NOT NULL DEFAULT 0,
    storage_key TEXT NOT NULL,
    status TEXT NOT NULL,
    output_formats TEXT[] NOT NULL DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS jobs (
    id UUID PRIMARY KEY,
    document_id UUID NOT NULL REFERENCES documents(id),
    status TEXT NOT NULL,
    stage TEXT NOT NULL DEFAULT 'queued',
    progress INT NOT NULL DEFAULT 0,
    error_code TEXT NOT NULL DEFAULT '',
    error_msg TEXT NOT NULL DEFAULT '',
    attempts INT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_jobs_document_id ON jobs(document_id);

CREATE TABLE IF NOT EXISTS artifacts (
    id UUID PRIMARY KEY,
    document_id UUID NOT NULL REFERENCES documents(id),
    job_id UUID NOT NULL REFERENCES jobs(id),
    kind TEXT NOT NULL,
    format TEXT NOT NULL,
    storage_key TEXT NOT NULL,
    size_bytes BIGINT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_artifacts_document_id ON artifacts(document_id);

-- +goose Down
DROP TABLE IF EXISTS artifacts;
DROP TABLE IF EXISTS jobs;
DROP TABLE IF EXISTS documents;
