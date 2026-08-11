package application_test

import (
	"bytes"
	"context"
	"testing"

	"github.com/thanhtai9606/DocForge/apps/api/internal/application"
	"github.com/thanhtai9606/DocForge/apps/api/internal/domain"
	"github.com/thanhtai9606/DocForge/apps/api/internal/infrastructure/memory"
)

func TestUploadDocumentHappyPath(t *testing.T) {
	queue := memory.NewQueue()
	svc := &application.Service{
		Documents: memory.NewDocumentRepo(),
		Jobs:      memory.NewJobRepo(),
		Artifacts: memory.NewArtifactRepo(),
		Objects:   memory.NewObjectStore(),
		Queue:     queue,
		Progress:  memory.NewProgress(),
		MaxBytes:  1024,
		MaxPages:  10,
	}
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
