package cleanup

import (
	"context"
	"log/slog"
	"time"

	"github.com/thanhtai9606/DocForge/apps/api/internal/domain"
	"github.com/thanhtai9606/DocForge/apps/api/internal/jobs"
	"github.com/thanhtai9606/DocForge/apps/api/internal/storage"
)

// Sweeper deletes expired document objects and metadata.
type Sweeper struct {
	Documents jobs.DocumentRepository
	Jobs      jobs.JobRepository
	Artifacts jobs.ArtifactRepository
	Objects   storage.ObjectStore
	TTL       time.Duration
	Logger    *slog.Logger
	Now       func() time.Time
}

func (s *Sweeper) RunOnce(ctx context.Context) (int, error) {
	if s.TTL <= 0 {
		return 0, nil
	}
	now := time.Now().UTC()
	if s.Now != nil {
		now = s.Now()
	}
	docs, err := s.Documents.ListRecent(ctx, 500)
	if err != nil {
		return 0, err
	}
	cutoff := now.Add(-s.TTL)
	removed := 0
	for i := range docs {
		doc := docs[i]
		if doc.CreatedAt.After(cutoff) {
			continue
		}
		if err := s.deleteDocument(ctx, &doc); err != nil {
			if s.Logger != nil {
				s.Logger.Warn("cleanup failed", "document_id", doc.ID, "error", err)
			}
			continue
		}
		removed++
	}
	return removed, nil
}

func (s *Sweeper) deleteDocument(ctx context.Context, doc *domain.Document) error {
	arts, err := s.Artifacts.ListByDocument(ctx, doc.ID)
	if err != nil {
		return err
	}
	for _, a := range arts {
		_ = s.Objects.Delete(ctx, a.StorageKey)
	}
	_ = s.Artifacts.DeleteByDocument(ctx, doc.ID)
	_ = s.Jobs.DeleteByDocument(ctx, doc.ID)
	_ = s.Objects.Delete(ctx, doc.StorageKey)
	return s.Documents.Delete(ctx, doc.ID)
}

// Start launches a background loop until ctx is cancelled.
func (s *Sweeper) Start(ctx context.Context, every time.Duration) {
	if every <= 0 {
		every = time.Hour
	}
	go func() {
		t := time.NewTicker(every)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				n, err := s.RunOnce(ctx)
				if s.Logger != nil {
					s.Logger.Info("cleanup sweep", "removed", n, "error", err)
				}
			}
		}
	}()
}
