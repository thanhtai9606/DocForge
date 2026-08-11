package jobs

import (
	"context"

	"github.com/thanhtai9606/DocForge/apps/api/internal/domain"
)

// DocumentRepository persists document metadata.
type DocumentRepository interface {
	Create(ctx context.Context, doc *domain.Document) error
	Get(ctx context.Context, id string) (*domain.Document, error)
	Update(ctx context.Context, doc *domain.Document) error
	ListRecent(ctx context.Context, limit int) ([]domain.Document, error)
}

// JobRepository persists job state.
type JobRepository interface {
	Create(ctx context.Context, job *domain.Job) error
	Get(ctx context.Context, id string) (*domain.Job, error)
	Update(ctx context.Context, job *domain.Job) error
}

// ArtifactRepository persists artifact metadata.
type ArtifactRepository interface {
	Create(ctx context.Context, artifact *domain.Artifact) error
	ListByDocument(ctx context.Context, documentID string) ([]domain.Artifact, error)
	Get(ctx context.Context, id string) (*domain.Artifact, error)
}

// QueuePublisher publishes work messages.
type QueuePublisher interface {
	PublishProcessJob(ctx context.Context, jobID, documentID string) error
}

// ProgressStore mirrors job progress for fast reads.
type ProgressStore interface {
	SetProgress(ctx context.Context, jobID string, stage string, progress int) error
	GetProgress(ctx context.Context, jobID string) (stage string, progress int, ok bool, err error)
}
