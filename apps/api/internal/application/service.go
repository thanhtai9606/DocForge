package application

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"path"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/thanhtai9606/DocForge/apps/api/internal/domain"
	"github.com/thanhtai9606/DocForge/apps/api/internal/jobs"
	"github.com/thanhtai9606/DocForge/apps/api/internal/storage"
)

// Service implements document/job use cases.
type Service struct {
	Documents   jobs.DocumentRepository
	Jobs        jobs.JobRepository
	Artifacts   jobs.ArtifactRepository
	Objects     storage.ObjectStore
	Queue       jobs.QueuePublisher
	Progress    jobs.ProgressStore
	MaxBytes    int64
	MaxPages    int
	MaxAttempts int
}

type UploadInput struct {
	Filename      string
	ContentType   string
	SizeBytes     int64
	Body          io.Reader
	OutputFormats []string
}

type UploadResult struct {
	DocumentID string
	JobID      string
	Status     domain.JobStatus
}

func (s *Service) maxAttempts() int {
	if s.MaxAttempts < 1 {
		return 3
	}
	return s.MaxAttempts
}

func (s *Service) UploadDocument(ctx context.Context, in UploadInput) (*UploadResult, error) {
	if in.SizeBytes > s.MaxBytes {
		return nil, domain.NewAppError(domain.CodeFileTooLarge, "uploaded file exceeds size limit", false)
	}
	filename := sanitizeFilename(in.Filename)
	if filename == "" || !strings.HasSuffix(strings.ToLower(filename), ".pdf") {
		return nil, domain.NewAppError(domain.CodeInvalidPDF, "only .pdf uploads are accepted", false)
	}
	payload, err := io.ReadAll(io.LimitReader(in.Body, s.MaxBytes+1))
	if err != nil {
		return nil, domain.NewAppError(domain.CodeStorageError, "failed to read upload body", true)
	}
	if int64(len(payload)) > s.MaxBytes {
		return nil, domain.NewAppError(domain.CodeFileTooLarge, "uploaded file exceeds size limit", false)
	}
	if !hasPDFMagic(payload) {
		return nil, domain.NewAppError(domain.CodeInvalidPDF, "the uploaded file is not a valid PDF", false)
	}

	contentType := strings.TrimSpace(in.ContentType)
	if contentType == "" || strings.EqualFold(contentType, "application/octet-stream") {
		contentType = "application/pdf"
	}
	if !isPDFContentType(contentType) {
		return nil, domain.NewAppError(domain.CodeInvalidPDF, "the uploaded file is not a valid PDF", false)
	}

	now := time.Now().UTC()
	docID := uuid.NewString()
	jobID := uuid.NewString()
	storageKey := path.Join("documents", docID, "original.pdf")

	formats := in.OutputFormats
	if len(formats) == 0 {
		formats = []string{"markdown", "docx", "json"}
	}

	if err := s.Objects.Put(ctx, storageKey, bytes.NewReader(payload), int64(len(payload)), contentType); err != nil {
		return nil, err
	}

	doc := &domain.Document{
		ID:            docID,
		Filename:      filename,
		ContentType:   contentType,
		SizeBytes:     int64(len(payload)),
		PageCount:     0,
		StorageKey:    storageKey,
		Status:        domain.DocumentQueued,
		OutputFormats: formats,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	if err := s.Documents.Create(ctx, doc); err != nil {
		return nil, err
	}

	job := &domain.Job{
		ID:         jobID,
		DocumentID: docID,
		Status:     domain.JobQueued,
		Stage:      "queued",
		Progress:   0,
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	if err := s.Jobs.Create(ctx, job); err != nil {
		return nil, err
	}

	if s.Progress != nil {
		_ = s.Progress.SetProgress(ctx, jobID, job.Stage, job.Progress)
	}
	if err := s.Queue.PublishProcessJob(ctx, jobID, docID); err != nil {
		job.Status = domain.JobFailed
		job.ErrorCode = domain.CodeInternal
		job.ErrorMsg = fmt.Sprintf("failed to enqueue job: %v", err)
		job.UpdatedAt = time.Now().UTC()
		_ = s.Jobs.Update(ctx, job)
		return nil, domain.NewAppError(domain.CodeInternal, "failed to enqueue processing job", true)
	}

	return &UploadResult{DocumentID: docID, JobID: jobID, Status: domain.JobQueued}, nil
}

func (s *Service) ListDocuments(ctx context.Context, limit int) ([]domain.Document, error) {
	if limit <= 0 {
		limit = 50
	}
	return s.Documents.ListRecent(ctx, limit)
}

func (s *Service) GetJob(ctx context.Context, jobID string) (*domain.Job, error) {
	job, err := s.Jobs.Get(ctx, jobID)
	if err != nil {
		return nil, err
	}
	if s.Progress != nil {
		if stage, progress, ok, perr := s.Progress.GetProgress(ctx, jobID); perr == nil && ok {
			job.Stage = stage
			job.Progress = progress
		}
	}
	return job, nil
}

func (s *Service) LatestJobForDocument(ctx context.Context, documentID string) (*domain.Job, error) {
	if _, err := s.Documents.Get(ctx, documentID); err != nil {
		return nil, err
	}
	job, err := s.Jobs.LatestByDocument(ctx, documentID)
	if err != nil {
		return nil, err
	}
	return s.GetJob(ctx, job.ID)
}

func (s *Service) GetDocument(ctx context.Context, documentID string) (*domain.Document, error) {
	return s.Documents.Get(ctx, documentID)
}

func (s *Service) DeleteDocument(ctx context.Context, documentID string) error {
	doc, err := s.Documents.Get(ctx, documentID)
	if err != nil {
		return err
	}
	arts, err := s.Artifacts.ListByDocument(ctx, documentID)
	if err != nil {
		return err
	}
	for _, a := range arts {
		_ = s.Objects.Delete(ctx, a.StorageKey)
	}
	_ = s.Artifacts.DeleteByDocument(ctx, documentID)
	_ = s.Jobs.DeleteByDocument(ctx, documentID)
	_ = s.Objects.Delete(ctx, doc.StorageKey)
	return s.Documents.Delete(ctx, documentID)
}

func (s *Service) ListArtifacts(ctx context.Context, documentID string) ([]domain.Artifact, error) {
	if _, err := s.Documents.Get(ctx, documentID); err != nil {
		return nil, err
	}
	return s.Artifacts.ListByDocument(ctx, documentID)
}

func (s *Service) GetArtifact(ctx context.Context, artifactID string) (*domain.Artifact, error) {
	return s.Artifacts.Get(ctx, artifactID)
}

func (s *Service) OpenArtifact(ctx context.Context, artifactID string) (*domain.Artifact, io.ReadCloser, error) {
	artifact, err := s.Artifacts.Get(ctx, artifactID)
	if err != nil {
		return nil, nil, err
	}
	rc, err := s.Objects.Get(ctx, artifact.StorageKey)
	if err != nil {
		return nil, nil, err
	}
	return artifact, rc, nil
}

func (s *Service) OpenOriginal(ctx context.Context, documentID string) (*domain.Document, io.ReadCloser, error) {
	doc, err := s.Documents.Get(ctx, documentID)
	if err != nil {
		return nil, nil, err
	}
	rc, err := s.Objects.Get(ctx, doc.StorageKey)
	if err != nil {
		return nil, nil, err
	}
	return doc, rc, nil
}

func (s *Service) CancelJob(ctx context.Context, jobID string) (*domain.Job, error) {
	job, err := s.Jobs.Get(ctx, jobID)
	if err != nil {
		return nil, err
	}
	if job.Status == domain.JobCompleted || job.Status == domain.JobFailed || job.Status == domain.JobCancelled {
		return job, nil
	}
	if err := job.Transition(domain.JobCancelled); err != nil {
		return nil, err
	}
	job.ErrorCode = domain.CodeJobCancelled
	job.ErrorMsg = "job cancelled by request"
	job.Stage = "cancelled"
	if err := s.Jobs.Update(ctx, job); err != nil {
		return nil, err
	}
	doc, err := s.Documents.Get(ctx, job.DocumentID)
	if err == nil {
		doc.Status = domain.DocumentCancelled
		doc.UpdatedAt = time.Now().UTC()
		_ = s.Documents.Update(ctx, doc)
	}
	if s.Progress != nil {
		_ = s.Progress.SetProgress(ctx, job.ID, job.Stage, job.Progress)
	}
	return job, nil
}

func (s *Service) RetryJob(ctx context.Context, jobID string) (*domain.Job, error) {
	old, err := s.Jobs.Get(ctx, jobID)
	if err != nil {
		return nil, err
	}
	if old.Status != domain.JobFailed && old.Status != domain.JobCancelled {
		return nil, domain.NewAppError(domain.CodeInternal, "only failed or cancelled jobs can be retried", false)
	}
	if old.Attempts+1 >= s.maxAttempts() {
		return nil, domain.NewAppError(domain.CodeInternal, "max retry attempts exceeded", false)
	}
	doc, err := s.Documents.Get(ctx, old.DocumentID)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	job := &domain.Job{
		ID:         uuid.NewString(),
		DocumentID: old.DocumentID,
		Status:     domain.JobQueued,
		Stage:      "queued",
		Progress:   0,
		Attempts:   old.Attempts + 1,
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	if err := s.Jobs.Create(ctx, job); err != nil {
		return nil, err
	}
	doc.Status = domain.DocumentQueued
	doc.UpdatedAt = now
	_ = s.Documents.Update(ctx, doc)
	if s.Progress != nil {
		_ = s.Progress.SetProgress(ctx, job.ID, job.Stage, job.Progress)
	}
	if err := s.Queue.PublishProcessJob(ctx, job.ID, job.DocumentID); err != nil {
		job.Status = domain.JobFailed
		job.ErrorCode = domain.CodeInternal
		job.ErrorMsg = fmt.Sprintf("failed to enqueue retry: %v", err)
		job.UpdatedAt = time.Now().UTC()
		_ = s.Jobs.Update(ctx, job)
		return nil, domain.NewAppError(domain.CodeInternal, "failed to enqueue retry job", true)
	}
	return job, nil
}

func sanitizeFilename(name string) string {
	name = path.Base(strings.ReplaceAll(name, "\\", "/"))
	name = strings.TrimSpace(name)
	return name
}

func isPDFContentType(ct string) bool {
	ct = strings.ToLower(strings.TrimSpace(ct))
	return ct == "application/pdf" || ct == "application/x-pdf" || strings.HasPrefix(ct, "application/pdf;")
}

func hasPDFMagic(b []byte) bool {
	return len(b) >= 4 && string(b[:4]) == "%PDF"
}
