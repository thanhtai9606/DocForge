package application_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/thanhtai9606/DocForge/apps/api/internal/application"
	"github.com/thanhtai9606/DocForge/apps/api/internal/domain"
	"github.com/thanhtai9606/DocForge/apps/api/internal/infrastructure/memory"
)

type failQueue struct{}

func (failQueue) PublishProcessJob(context.Context, string, string) error {
	return errors.New("broker down")
}

func newService(t *testing.T) (*application.Service, *memory.Queue, *memory.Progress, *memory.ObjectStore, *memory.ArtifactRepo) {
	t.Helper()
	queue := memory.NewQueue()
	progress := memory.NewProgress()
	objects := memory.NewObjectStore()
	artifacts := memory.NewArtifactRepo()
	svc := &application.Service{
		Documents: memory.NewDocumentRepo(),
		Jobs:      memory.NewJobRepo(),
		Artifacts: artifacts,
		Objects:   objects,
		Queue:     queue,
		Progress:  progress,
		MaxBytes:  1024,
		MaxPages:  10,
	}
	return svc, queue, progress, objects, artifacts
}

func TestUploadDocumentHappyPath(t *testing.T) {
	svc, queue, _, _, _ := newService(t)
	res, err := svc.UploadDocument(context.Background(), application.UploadInput{
		Filename:    "demo.pdf",
		ContentType: "application/pdf",
		SizeBytes:   10,
		Body:        bytes.NewReader([]byte("%PDF-1.7\ndata")),
	})
	if err != nil {
		t.Fatalf("upload: %v", err)
	}
	if res.Status != domain.JobQueued {
		t.Fatalf("status=%s", res.Status)
	}
	if len(queue.Messages) != 1 {
		t.Fatalf("expected 1 queued message, got %d", len(queue.Messages))
	}
	job, err := svc.GetJob(context.Background(), res.JobID)
	if err != nil {
		t.Fatal(err)
	}
	if job.Stage != "queued" {
		t.Fatalf("stage=%s", job.Stage)
	}
}

func TestUploadRejectsNonPDFExtension(t *testing.T) {
	svc, _, _, _, _ := newService(t)
	_, err := svc.UploadDocument(context.Background(), application.UploadInput{
		Filename: "note.txt",
		Body:     bytes.NewReader([]byte("%PDF-1.7")),
	})
	assertAppCode(t, err, domain.CodeInvalidPDF)
}

func TestUploadRejectsMissingMagic(t *testing.T) {
	svc, _, _, _, _ := newService(t)
	_, err := svc.UploadDocument(context.Background(), application.UploadInput{
		Filename: "x.pdf",
		Body:     bytes.NewReader([]byte("not-a-pdf")),
	})
	assertAppCode(t, err, domain.CodeInvalidPDF)
}

func TestUploadRejectsTooLargeDeclaredSize(t *testing.T) {
	svc, _, _, _, _ := newService(t)
	_, err := svc.UploadDocument(context.Background(), application.UploadInput{
		Filename:  "x.pdf",
		SizeBytes: 2048,
		Body:      bytes.NewReader([]byte("%PDF-1.7")),
	})
	assertAppCode(t, err, domain.CodeFileTooLarge)
}

func TestUploadSanitizesPathFilename(t *testing.T) {
	svc, _, _, _, _ := newService(t)
	res, err := svc.UploadDocument(context.Background(), application.UploadInput{
		Filename: `..\..\evil/report.PDF`,
		Body:     bytes.NewReader([]byte("%PDF-1.4")),
	})
	if err != nil {
		t.Fatal(err)
	}
	doc, err := svc.GetDocument(context.Background(), res.DocumentID)
	if err != nil {
		t.Fatal(err)
	}
	if doc.Filename != "report.PDF" {
		t.Fatalf("filename=%q", doc.Filename)
	}
}

func TestUploadDefaultOutputFormats(t *testing.T) {
	svc, _, _, _, _ := newService(t)
	res, err := svc.UploadDocument(context.Background(), application.UploadInput{
		Filename: "a.pdf",
		Body:     bytes.NewReader([]byte("%PDF-1.4")),
	})
	if err != nil {
		t.Fatal(err)
	}
	doc, err := svc.GetDocument(context.Background(), res.DocumentID)
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.OutputFormats) != 3 {
		t.Fatalf("formats=%v", doc.OutputFormats)
	}
}

func TestUploadOctetStreamAllowedWithMagic(t *testing.T) {
	svc, _, _, _, _ := newService(t)
	_, err := svc.UploadDocument(context.Background(), application.UploadInput{
		Filename:    "a.pdf",
		ContentType: "application/octet-stream",
		Body:        bytes.NewReader([]byte("%PDF-1.4")),
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestUploadQueueFailureMarksJobFailed(t *testing.T) {
	jobs := memory.NewJobRepo()
	rec := &recordingJobs{JobRepo: jobs}
	svc := &application.Service{
		Documents: memory.NewDocumentRepo(),
		Jobs:      rec,
		Artifacts: memory.NewArtifactRepo(),
		Objects:   memory.NewObjectStore(),
		Queue:     failQueue{},
		Progress:  memory.NewProgress(),
		MaxBytes:  1024,
	}
	_, err := svc.UploadDocument(context.Background(), application.UploadInput{
		Filename: "a.pdf",
		Body:     bytes.NewReader([]byte("%PDF-1.4")),
	})
	assertAppCode(t, err, domain.CodeInternal)
	if rec.last == nil || rec.last.Status != domain.JobFailed || rec.last.ErrorCode != domain.CodeInternal {
		t.Fatalf("expected failed job update, got %+v", rec.last)
	}
}

type recordingJobs struct {
	*memory.JobRepo
	last *domain.Job
}

func (r *recordingJobs) Create(ctx context.Context, job *domain.Job) error {
	cp := *job
	r.last = &cp
	return r.JobRepo.Create(ctx, job)
}

func (r *recordingJobs) Update(ctx context.Context, job *domain.Job) error {
	cp := *job
	r.last = &cp
	return r.JobRepo.Update(ctx, job)
}

func TestGetJobPrefersRedisProgress(t *testing.T) {
	svc, _, progress, _, _ := newService(t)
	res, err := svc.UploadDocument(context.Background(), application.UploadInput{
		Filename: "a.pdf",
		Body:     bytes.NewReader([]byte("%PDF-1.4")),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := progress.SetProgress(context.Background(), res.JobID, "ocr", 42); err != nil {
		t.Fatal(err)
	}
	job, err := svc.GetJob(context.Background(), res.JobID)
	if err != nil {
		t.Fatal(err)
	}
	if job.Stage != "ocr" || job.Progress != 42 {
		t.Fatalf("job=%+v", job)
	}
}

func TestCancelJob(t *testing.T) {
	svc, _, _, _, _ := newService(t)
	res, err := svc.UploadDocument(context.Background(), application.UploadInput{
		Filename: "a.pdf",
		Body:     bytes.NewReader([]byte("%PDF-1.4")),
	})
	if err != nil {
		t.Fatal(err)
	}
	job, err := svc.CancelJob(context.Background(), res.JobID)
	if err != nil {
		t.Fatal(err)
	}
	if job.Status != domain.JobCancelled || job.ErrorCode != domain.CodeJobCancelled {
		t.Fatalf("job=%+v", job)
	}
	doc, err := svc.GetDocument(context.Background(), res.DocumentID)
	if err != nil {
		t.Fatal(err)
	}
	if doc.Status != domain.DocumentCancelled {
		t.Fatalf("doc status=%s", doc.Status)
	}
	// idempotent cancel
	job2, err := svc.CancelJob(context.Background(), res.JobID)
	if err != nil || job2.Status != domain.JobCancelled {
		t.Fatalf("second cancel: job=%+v err=%v", job2, err)
	}
}

func TestArtifactsFlow(t *testing.T) {
	svc, _, _, objects, artifacts := newService(t)
	res, err := svc.UploadDocument(context.Background(), application.UploadInput{
		Filename: "a.pdf",
		Body:     bytes.NewReader([]byte("%PDF-1.4")),
	})
	if err != nil {
		t.Fatal(err)
	}
	key := "artifacts/" + res.DocumentID + "/out.md"
	if err := objects.Put(context.Background(), key, strings.NewReader("# hi"), 4, "text/markdown"); err != nil {
		t.Fatal(err)
	}
	art := &domain.Artifact{
		ID:         "art-1",
		DocumentID: res.DocumentID,
		JobID:      res.JobID,
		Kind:       "export",
		Format:     "markdown",
		StorageKey: key,
		SizeBytes:  4,
		CreatedAt:  time.Now().UTC(),
	}
	if err := artifacts.Create(context.Background(), art); err != nil {
		t.Fatal(err)
	}
	list, err := svc.ListArtifacts(context.Background(), res.DocumentID)
	if err != nil || len(list) != 1 {
		t.Fatalf("list=%v err=%v", list, err)
	}
	got, err := svc.GetArtifact(context.Background(), "art-1")
	if err != nil || got.Format != "markdown" {
		t.Fatalf("get=%+v err=%v", got, err)
	}
	_, rc, err := svc.OpenArtifact(context.Background(), "art-1")
	if err != nil {
		t.Fatal(err)
	}
	defer rc.Close()
	body, _ := io.ReadAll(rc)
	if string(body) != "# hi" {
		t.Fatalf("body=%q", body)
	}
}

func TestListArtifactsMissingDocument(t *testing.T) {
	svc, _, _, _, _ := newService(t)
	_, err := svc.ListArtifacts(context.Background(), "missing")
	assertAppCode(t, err, domain.CodeNotFound)
}

func assertAppCode(t *testing.T, err error, code string) {
	t.Helper()
	appErr, ok := domain.AsAppError(err)
	if !ok {
		t.Fatalf("expected AppError, got %v", err)
	}
	if appErr.Code != code {
		t.Fatalf("code=%s want=%s msg=%s", appErr.Code, code, appErr.Message)
	}
}
