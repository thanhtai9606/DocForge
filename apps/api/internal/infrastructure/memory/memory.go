package memory

import (
	"bytes"
	"context"
	"io"
	"sync"
	"time"

	"github.com/thanhtai9606/DocForge/apps/api/internal/domain"
	"github.com/thanhtai9606/DocForge/apps/api/internal/storage"
)

// DocumentRepo is an in-memory DocumentRepository.
type DocumentRepo struct {
	mu   sync.RWMutex
	data map[string]*domain.Document
}

func NewDocumentRepo() *DocumentRepo {
	return &DocumentRepo{data: map[string]*domain.Document{}}
}

func (r *DocumentRepo) Create(_ context.Context, doc *domain.Document) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	cp := *doc
	r.data[doc.ID] = &cp
	return nil
}

func (r *DocumentRepo) Get(_ context.Context, id string) (*domain.Document, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	doc, ok := r.data[id]
	if !ok {
		return nil, domain.NewAppError(domain.CodeNotFound, "document not found", false)
	}
	cp := *doc
	return &cp, nil
}

func (r *DocumentRepo) Update(_ context.Context, doc *domain.Document) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.data[doc.ID]; !ok {
		return domain.NewAppError(domain.CodeNotFound, "document not found", false)
	}
	cp := *doc
	r.data[doc.ID] = &cp
	return nil
}

func (r *DocumentRepo) ListRecent(_ context.Context, limit int) ([]domain.Document, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]domain.Document, 0, len(r.data))
	for _, d := range r.data {
		out = append(out, *d)
	}
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

// JobRepo is an in-memory JobRepository.
type JobRepo struct {
	mu   sync.RWMutex
	data map[string]*domain.Job
}

func NewJobRepo() *JobRepo {
	return &JobRepo{data: map[string]*domain.Job{}}
}

func (r *JobRepo) Create(_ context.Context, job *domain.Job) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	cp := *job
	r.data[job.ID] = &cp
	return nil
}

func (r *JobRepo) Get(_ context.Context, id string) (*domain.Job, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	job, ok := r.data[id]
	if !ok {
		return nil, domain.NewAppError(domain.CodeNotFound, "job not found", false)
	}
	cp := *job
	return &cp, nil
}

func (r *JobRepo) Update(_ context.Context, job *domain.Job) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.data[job.ID]; !ok {
		return domain.NewAppError(domain.CodeNotFound, "job not found", false)
	}
	cp := *job
	cp.UpdatedAt = time.Now().UTC()
	r.data[job.ID] = &cp
	return nil
}

// ArtifactRepo is an in-memory ArtifactRepository.
type ArtifactRepo struct {
	mu   sync.RWMutex
	data map[string]*domain.Artifact
}

func NewArtifactRepo() *ArtifactRepo {
	return &ArtifactRepo{data: map[string]*domain.Artifact{}}
}

func (r *ArtifactRepo) Create(_ context.Context, artifact *domain.Artifact) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	cp := *artifact
	r.data[artifact.ID] = &cp
	return nil
}

func (r *ArtifactRepo) ListByDocument(_ context.Context, documentID string) ([]domain.Artifact, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]domain.Artifact, 0)
	for _, a := range r.data {
		if a.DocumentID == documentID {
			out = append(out, *a)
		}
	}
	return out, nil
}

func (r *ArtifactRepo) Get(_ context.Context, id string) (*domain.Artifact, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	a, ok := r.data[id]
	if !ok {
		return nil, domain.NewAppError(domain.CodeNotFound, "artifact not found", false)
	}
	cp := *a
	return &cp, nil
}

// ObjectStore is an in-memory ObjectStore.
type ObjectStore struct {
	mu   sync.RWMutex
	data map[string][]byte
	meta map[string]string
}

func NewObjectStore() *ObjectStore {
	return &ObjectStore{data: map[string][]byte{}, meta: map[string]string{}}
}

var _ storage.ObjectStore = (*ObjectStore)(nil)

func (s *ObjectStore) Put(_ context.Context, key string, r io.Reader, _ int64, contentType string) error {
	b, err := io.ReadAll(r)
	if err != nil {
		return domain.NewAppError(domain.CodeStorageError, "failed to read upload", true)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data[key] = b
	s.meta[key] = contentType
	return nil
}

func (s *ObjectStore) Get(_ context.Context, key string) (io.ReadCloser, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	b, ok := s.data[key]
	if !ok {
		return nil, domain.NewAppError(domain.CodeNotFound, "object not found", false)
	}
	cp := make([]byte, len(b))
	copy(cp, b)
	return io.NopCloser(bytes.NewReader(cp)), nil
}

func (s *ObjectStore) Delete(_ context.Context, key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.data, key)
	delete(s.meta, key)
	return nil
}

// Queue captures published jobs.
type Queue struct {
	mu      sync.Mutex
	Messages []struct{ JobID, DocumentID string }
}

func NewQueue() *Queue { return &Queue{} }

func (q *Queue) PublishProcessJob(_ context.Context, jobID, documentID string) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.Messages = append(q.Messages, struct{ JobID, DocumentID string }{jobID, documentID})
	return nil
}

// Progress is an in-memory progress store.
type Progress struct {
	mu   sync.RWMutex
	data map[string]struct {
		Stage    string
		Progress int
	}
}

func NewProgress() *Progress {
	return &Progress{data: map[string]struct {
		Stage    string
		Progress int
	}{}}
}

func (p *Progress) SetProgress(_ context.Context, jobID, stage string, progress int) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.data[jobID] = struct {
		Stage    string
		Progress int
	}{Stage: stage, Progress: progress}
	return nil
}

func (p *Progress) GetProgress(_ context.Context, jobID string) (string, int, bool, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	v, ok := p.data[jobID]
	if !ok {
		return "", 0, false, nil
	}
	return v.Stage, v.Progress, true, nil
}
