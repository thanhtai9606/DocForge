package domain

import (
	"errors"
	"time"
)

// JobStatus is the durable lifecycle state of a processing job.
type JobStatus string

const (
	JobQueued     JobStatus = "queued"
	JobProcessing JobStatus = "processing"
	JobCompleted  JobStatus = "completed"
	JobFailed     JobStatus = "failed"
	JobCancelled  JobStatus = "cancelled"
)

// DocumentStatus mirrors high-level document processing state.
type DocumentStatus string

const (
	DocumentQueued     DocumentStatus = "queued"
	DocumentProcessing DocumentStatus = "processing"
	DocumentCompleted  DocumentStatus = "completed"
	DocumentFailed     DocumentStatus = "failed"
	DocumentCancelled  DocumentStatus = "cancelled"
)

// Document is durable metadata for an uploaded PDF.
type Document struct {
	ID           string
	Filename     string
	ContentType  string
	SizeBytes    int64
	PageCount    int
	StorageKey   string
	Status       DocumentStatus
	OutputFormats []string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// Job tracks processing progress for a document.
type Job struct {
	ID         string
	DocumentID string
	Status     JobStatus
	Stage      string
	Progress   int
	ErrorCode  string
	ErrorMsg   string
	Attempts   int
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

// Artifact is an exported or intermediate object stored in object storage.
type Artifact struct {
	ID         string
	DocumentID string
	JobID      string
	Kind       string
	Format     string
	StorageKey string
	SizeBytes  int64
	CreatedAt  time.Time
}

// Error codes from PROJECT_SPEC §6.
const (
	CodeInvalidPDF            = "INVALID_PDF"
	CodeUnsupportedDocument   = "UNSUPPORTED_DOCUMENT"
	CodeFileTooLarge          = "FILE_TOO_LARGE"
	CodePageLimitExceeded     = "PAGE_LIMIT_EXCEEDED"
	CodeOCRTimeout            = "OCR_TIMEOUT"
	CodeOCRProviderError      = "OCR_PROVIDER_ERROR"
	CodeLayoutFailed          = "LAYOUT_FAILED"
	CodeNormalizationFailed   = "NORMALIZATION_FAILED"
	CodeExportFailed          = "EXPORT_FAILED"
	CodeStorageError          = "STORAGE_ERROR"
	CodeJobCancelled          = "JOB_CANCELLED"
	CodeNotFound              = "NOT_FOUND"
	CodeInternal              = "INTERNAL_ERROR"
	CodeUnauthorized          = "UNAUTHORIZED"
	CodeRateLimited           = "RATE_LIMITED"
	CodeQuotaExceeded         = "QUOTA_EXCEEDED"
)

// AppError is a domain error with API mapping metadata.
type AppError struct {
	Code      string
	Message   string
	Retryable bool
}

func (e *AppError) Error() string {
	return e.Message
}

func NewAppError(code, message string, retryable bool) *AppError {
	return &AppError{Code: code, Message: message, Retryable: retryable}
}

func AsAppError(err error) (*AppError, bool) {
	var appErr *AppError
	if errors.As(err, &appErr) {
		return appErr, true
	}
	return nil, false
}

// CanTransition reports whether a job may move from -> to.
func CanTransition(from, to JobStatus) bool {
	switch from {
	case JobQueued:
		return to == JobProcessing || to == JobCancelled || to == JobFailed
	case JobProcessing:
		return to == JobCompleted || to == JobFailed || to == JobCancelled
	default:
		return false
	}
}

// Transition updates job status when allowed.
func (j *Job) Transition(to JobStatus) error {
	if !CanTransition(j.Status, to) {
		return NewAppError(CodeInternal, "invalid job status transition", false)
	}
	j.Status = to
	j.UpdatedAt = time.Now().UTC()
	return nil
}
