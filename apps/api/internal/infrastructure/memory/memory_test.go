package memory_test

import (
	"bytes"
	"context"
	"io"
	"testing"
	"time"

	"github.com/thanhtai9606/DocForge/apps/api/internal/domain"
	"github.com/thanhtai9606/DocForge/apps/api/internal/infrastructure/memory"
)

func TestDocumentRepoCRUD(t *testing.T) {
	repo := memory.NewDocumentRepo()
	ctx := context.Background()
	doc := &domain.Document{
		ID:        "d1",
		Filename:  "a.pdf",
		Status:    domain.DocumentQueued,
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}
	if err := repo.Create(ctx, doc); err != nil {
		t.Fatal(err)
	}
	got, err := repo.Get(ctx, "d1")
	if err != nil || got.Filename != "a.pdf" {
		t.Fatalf("get=%+v err=%v", got, err)
	}
	got.Status = domain.DocumentCompleted
	if err := repo.Update(ctx, got); err != nil {
		t.Fatal(err)
	}
	list, err := repo.ListRecent(ctx, 10)
	if err != nil || len(list) != 1 || list[0].Status != domain.DocumentCompleted {
		t.Fatalf("list=%v err=%v", list, err)
	}
	if _, err := repo.Get(ctx, "missing"); err == nil {
		t.Fatal("expected not found")
	}
}

func TestJobRepoCRUD(t *testing.T) {
	repo := memory.NewJobRepo()
	ctx := context.Background()
	job := &domain.Job{ID: "j1", DocumentID: "d1", Status: domain.JobQueued, Stage: "queued"}
	if err := repo.Create(ctx, job); err != nil {
		t.Fatal(err)
	}
	got, err := repo.Get(ctx, "j1")
	if err != nil {
		t.Fatal(err)
	}
	got.Status = domain.JobProcessing
	got.Stage = "ocr"
	got.Progress = 10
	if err := repo.Update(ctx, got); err != nil {
		t.Fatal(err)
	}
	got, err = repo.Get(ctx, "j1")
	if err != nil || got.Progress != 10 || got.Stage != "ocr" {
		t.Fatalf("got=%+v err=%v", got, err)
	}
}

func TestArtifactRepo(t *testing.T) {
	repo := memory.NewArtifactRepo()
	ctx := context.Background()
	a1 := &domain.Artifact{ID: "a1", DocumentID: "d1", JobID: "j1", Format: "json"}
	a2 := &domain.Artifact{ID: "a2", DocumentID: "d2", JobID: "j2", Format: "md"}
	_ = repo.Create(ctx, a1)
	_ = repo.Create(ctx, a2)
	list, err := repo.ListByDocument(ctx, "d1")
	if err != nil || len(list) != 1 || list[0].ID != "a1" {
		t.Fatalf("list=%v err=%v", list, err)
	}
	got, err := repo.Get(ctx, "a2")
	if err != nil || got.Format != "md" {
		t.Fatalf("got=%+v err=%v", got, err)
	}
}

func TestObjectStoreRoundTrip(t *testing.T) {
	store := memory.NewObjectStore()
	ctx := context.Background()
	if err := store.Put(ctx, "k", bytes.NewReader([]byte("hello")), 5, "text/plain"); err != nil {
		t.Fatal(err)
	}
	rc, err := store.Get(ctx, "k")
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(rc)
	_ = rc.Close()
	if string(b) != "hello" {
		t.Fatalf("body=%q", b)
	}
	if err := store.Delete(ctx, "k"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Get(ctx, "k"); err == nil {
		t.Fatal("expected missing object")
	}
}

func TestProgressAndQueue(t *testing.T) {
	p := memory.NewProgress()
	ctx := context.Background()
	if err := p.SetProgress(ctx, "j1", "layout", 55); err != nil {
		t.Fatal(err)
	}
	stage, progress, ok, err := p.GetProgress(ctx, "j1")
	if err != nil || !ok || stage != "layout" || progress != 55 {
		t.Fatalf("stage=%s progress=%d ok=%v err=%v", stage, progress, ok, err)
	}
	_, _, ok, err = p.GetProgress(ctx, "missing")
	if err != nil || ok {
		t.Fatal("expected miss")
	}

	q := memory.NewQueue()
	if err := q.PublishProcessJob(ctx, "j1", "d1"); err != nil {
		t.Fatal(err)
	}
	if len(q.Messages) != 1 || q.Messages[0].JobID != "j1" {
		t.Fatalf("messages=%v", q.Messages)
	}
}
