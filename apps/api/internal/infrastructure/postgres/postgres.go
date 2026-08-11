package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/lib/pq"

	"github.com/thanhtai9606/DocForge/apps/api/internal/domain"
)

// Store wraps a SQL database connection.
type Store struct {
	db *sql.DB
}

func New(db *sql.DB) *Store { return &Store{db: db} }

func Open(databaseURL string) (*sql.DB, error) {
	db, err := sql.Open("postgres", databaseURL)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(10)
	db.SetConnMaxLifetime(time.Hour)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	return db, nil
}

type DocumentRepo struct{ store *Store }

func NewDocumentRepo(store *Store) *DocumentRepo { return &DocumentRepo{store: store} }

func (r *DocumentRepo) Create(ctx context.Context, doc *domain.Document) error {
	_, err := r.store.db.ExecContext(ctx, `
		INSERT INTO documents (id, filename, content_type, size_bytes, page_count, storage_key, status, output_formats, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`,
		doc.ID, doc.Filename, doc.ContentType, doc.SizeBytes, doc.PageCount, doc.StorageKey, doc.Status, pq.Array(doc.OutputFormats), doc.CreatedAt, doc.UpdatedAt,
	)
	return err
}

func (r *DocumentRepo) Get(ctx context.Context, id string) (*domain.Document, error) {
	row := r.store.db.QueryRowContext(ctx, `
		SELECT id, filename, content_type, size_bytes, page_count, storage_key, status, output_formats, created_at, updated_at
		FROM documents WHERE id=$1`, id)
	var doc domain.Document
	var formats pq.StringArray
	if err := row.Scan(&doc.ID, &doc.Filename, &doc.ContentType, &doc.SizeBytes, &doc.PageCount, &doc.StorageKey, &doc.Status, &formats, &doc.CreatedAt, &doc.UpdatedAt); err != nil {
		if err == sql.ErrNoRows {
			return nil, domain.NewAppError(domain.CodeNotFound, "document not found", false)
		}
		return nil, err
	}
	doc.OutputFormats = []string(formats)
	return &doc, nil
}

func (r *DocumentRepo) Update(ctx context.Context, doc *domain.Document) error {
	res, err := r.store.db.ExecContext(ctx, `
		UPDATE documents SET filename=$2, content_type=$3, size_bytes=$4, page_count=$5, storage_key=$6, status=$7, output_formats=$8, updated_at=$9
		WHERE id=$1`,
		doc.ID, doc.Filename, doc.ContentType, doc.SizeBytes, doc.PageCount, doc.StorageKey, doc.Status, pq.Array(doc.OutputFormats), doc.UpdatedAt,
	)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return domain.NewAppError(domain.CodeNotFound, "document not found", false)
	}
	return nil
}

func (r *DocumentRepo) ListRecent(ctx context.Context, limit int) ([]domain.Document, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := r.store.db.QueryContext(ctx, `
		SELECT id, filename, content_type, size_bytes, page_count, storage_key, status, output_formats, created_at, updated_at
		FROM documents ORDER BY created_at DESC LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]domain.Document, 0)
	for rows.Next() {
		var doc domain.Document
		var formats pq.StringArray
		if err := rows.Scan(&doc.ID, &doc.Filename, &doc.ContentType, &doc.SizeBytes, &doc.PageCount, &doc.StorageKey, &doc.Status, &formats, &doc.CreatedAt, &doc.UpdatedAt); err != nil {
			return nil, err
		}
		doc.OutputFormats = []string(formats)
		out = append(out, doc)
	}
	return out, rows.Err()
}

func (r *DocumentRepo) Delete(ctx context.Context, id string) error {
	res, err := r.store.db.ExecContext(ctx, `DELETE FROM documents WHERE id=$1`, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return domain.NewAppError(domain.CodeNotFound, "document not found", false)
	}
	return nil
}

type JobRepo struct{ store *Store }

func NewJobRepo(store *Store) *JobRepo { return &JobRepo{store: store} }

func (r *JobRepo) Create(ctx context.Context, job *domain.Job) error {
	_, err := r.store.db.ExecContext(ctx, `
		INSERT INTO jobs (id, document_id, status, stage, progress, error_code, error_msg, attempts, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`,
		job.ID, job.DocumentID, job.Status, job.Stage, job.Progress, job.ErrorCode, job.ErrorMsg, job.Attempts, job.CreatedAt, job.UpdatedAt,
	)
	return err
}

func (r *JobRepo) Get(ctx context.Context, id string) (*domain.Job, error) {
	row := r.store.db.QueryRowContext(ctx, `
		SELECT id, document_id, status, stage, progress, error_code, error_msg, attempts, created_at, updated_at
		FROM jobs WHERE id=$1`, id)
	var job domain.Job
	if err := row.Scan(&job.ID, &job.DocumentID, &job.Status, &job.Stage, &job.Progress, &job.ErrorCode, &job.ErrorMsg, &job.Attempts, &job.CreatedAt, &job.UpdatedAt); err != nil {
		if err == sql.ErrNoRows {
			return nil, domain.NewAppError(domain.CodeNotFound, "job not found", false)
		}
		return nil, err
	}
	return &job, nil
}

func (r *JobRepo) Update(ctx context.Context, job *domain.Job) error {
	res, err := r.store.db.ExecContext(ctx, `
		UPDATE jobs SET status=$2, stage=$3, progress=$4, error_code=$5, error_msg=$6, attempts=$7, updated_at=$8
		WHERE id=$1`,
		job.ID, job.Status, job.Stage, job.Progress, job.ErrorCode, job.ErrorMsg, job.Attempts, job.UpdatedAt,
	)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return domain.NewAppError(domain.CodeNotFound, "job not found", false)
	}
	return nil
}

func (r *JobRepo) LatestByDocument(ctx context.Context, documentID string) (*domain.Job, error) {
	row := r.store.db.QueryRowContext(ctx, `
		SELECT id, document_id, status, stage, progress, error_code, error_msg, attempts, created_at, updated_at
		FROM jobs WHERE document_id=$1 ORDER BY created_at DESC LIMIT 1`, documentID)
	var job domain.Job
	if err := row.Scan(&job.ID, &job.DocumentID, &job.Status, &job.Stage, &job.Progress, &job.ErrorCode, &job.ErrorMsg, &job.Attempts, &job.CreatedAt, &job.UpdatedAt); err != nil {
		if err == sql.ErrNoRows {
			return nil, domain.NewAppError(domain.CodeNotFound, "job not found", false)
		}
		return nil, err
	}
	return &job, nil
}

func (r *JobRepo) DeleteByDocument(ctx context.Context, documentID string) error {
	_, err := r.store.db.ExecContext(ctx, `DELETE FROM jobs WHERE document_id=$1`, documentID)
	return err
}

type ArtifactRepo struct{ store *Store }

func NewArtifactRepo(store *Store) *ArtifactRepo { return &ArtifactRepo{store: store} }

func (r *ArtifactRepo) Create(ctx context.Context, artifact *domain.Artifact) error {
	_, err := r.store.db.ExecContext(ctx, `
		INSERT INTO artifacts (id, document_id, job_id, kind, format, storage_key, size_bytes, created_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`,
		artifact.ID, artifact.DocumentID, artifact.JobID, artifact.Kind, artifact.Format, artifact.StorageKey, artifact.SizeBytes, artifact.CreatedAt,
	)
	return err
}

func (r *ArtifactRepo) ListByDocument(ctx context.Context, documentID string) ([]domain.Artifact, error) {
	rows, err := r.store.db.QueryContext(ctx, `
		SELECT id, document_id, job_id, kind, format, storage_key, size_bytes, created_at
		FROM artifacts WHERE document_id=$1 ORDER BY created_at ASC`, documentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]domain.Artifact, 0)
	for rows.Next() {
		var a domain.Artifact
		if err := rows.Scan(&a.ID, &a.DocumentID, &a.JobID, &a.Kind, &a.Format, &a.StorageKey, &a.SizeBytes, &a.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func (r *ArtifactRepo) Get(ctx context.Context, id string) (*domain.Artifact, error) {
	row := r.store.db.QueryRowContext(ctx, `
		SELECT id, document_id, job_id, kind, format, storage_key, size_bytes, created_at
		FROM artifacts WHERE id=$1`, id)
	var a domain.Artifact
	if err := row.Scan(&a.ID, &a.DocumentID, &a.JobID, &a.Kind, &a.Format, &a.StorageKey, &a.SizeBytes, &a.CreatedAt); err != nil {
		if err == sql.ErrNoRows {
			return nil, domain.NewAppError(domain.CodeNotFound, "artifact not found", false)
		}
		return nil, err
	}
	return &a, nil
}

func (r *ArtifactRepo) DeleteByDocument(ctx context.Context, documentID string) error {
	_, err := r.store.db.ExecContext(ctx, `DELETE FROM artifacts WHERE document_id=$1`, documentID)
	return err
}

// EnsureSchema applies the bootstrap migration for local/dev usage.
func EnsureSchema(ctx context.Context, db *sql.DB) error {
	_, err := db.ExecContext(ctx, `
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
    document_id UUID NOT NULL REFERENCES documents(id) ON DELETE CASCADE,
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
    document_id UUID NOT NULL REFERENCES documents(id) ON DELETE CASCADE,
    job_id UUID NOT NULL REFERENCES jobs(id) ON DELETE CASCADE,
    kind TEXT NOT NULL,
    format TEXT NOT NULL,
    storage_key TEXT NOT NULL,
    size_bytes BIGINT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_artifacts_document_id ON artifacts(document_id);
`)
	if err != nil {
		return fmt.Errorf("ensure schema: %w", err)
	}
	return nil
}
