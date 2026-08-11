package orchestrator_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"testing"
	"time"

	"github.com/thanhtai9606/DocForge/apps/api/internal/domain"
	"github.com/thanhtai9606/DocForge/apps/api/internal/infrastructure/memory"
	"github.com/thanhtai9606/DocForge/apps/api/internal/orchestrator"
	"github.com/thanhtai9606/DocForge/apps/api/internal/providers/stubocr"
	"github.com/thanhtai9606/DocForge/packages/cdom"
)

func TestDetectDigitalPDF(t *testing.T) {
	pdf := []byte("%PDF-1.4\n1 0 obj\n<< /Type /Page >>\nendobj\n(BT Hello Vietnamese Xin chào ET)\n(More digital text content here for detection threshold padding.)\n")
	kind, pages, _, err := orchestrator.DetectAndExtract(pdf)
	if err != nil {
		t.Fatal(err)
	}
	if pages < 1 {
		t.Fatalf("pages=%d", pages)
	}
	if kind != orchestrator.KindDigital && kind != orchestrator.KindMixed {
		t.Fatalf("kind=%s", kind)
	}
}

func TestDetectScannedPDF(t *testing.T) {
	pdf := []byte("%PDF-1.4\n% scanned binary-ish content with almost no text strings\n1 0 obj<< /Type /Page >>endobj\n")
	kind, _, blocks, err := orchestrator.DetectAndExtract(pdf)
	if err != nil {
		t.Fatal(err)
	}
	if kind != orchestrator.KindScanned {
		t.Fatalf("kind=%s", kind)
	}
	if len(blocks[0]) > 0 {
		t.Fatalf("expected empty extract for scanned, got %#v", blocks[0])
	}
}

func TestEngineDigitalPipeline(t *testing.T) {
	ctx := context.Background()
	docs := memory.NewDocumentRepo()
	jobs := memory.NewJobRepo()
	artifacts := memory.NewArtifactRepo()
	objects := memory.NewObjectStore()
	progress := memory.NewProgress()

	pdf := []byte("%PDF-1.4\n1 0 obj<< /Type /Page >>endobj\n(Paragraph one with enough characters for digital classification threshold.)\n(Paragraph two continues the sample digital document text body.)\n")
	docID := "doc-digital"
	jobID := "job-digital"
	key := "documents/" + docID + "/original.pdf"
	_ = objects.Put(ctx, key, bytes.NewReader(pdf), int64(len(pdf)), "application/pdf")
	now := time.Now().UTC()
	_ = docs.Create(ctx, &domain.Document{
		ID: docID, Filename: "digital.pdf", ContentType: "application/pdf",
		StorageKey: key, Status: domain.DocumentQueued, CreatedAt: now, UpdatedAt: now,
	})
	_ = jobs.Create(ctx, &domain.Job{
		ID: jobID, DocumentID: docID, Status: domain.JobQueued, Stage: "queued", CreatedAt: now, UpdatedAt: now,
	})

	engine := &orchestrator.Engine{
		Documents: docs, Jobs: jobs, Artifacts: artifacts, Objects: objects, Progress: progress,
		OCR: stubocr.New(),
	}
	res, err := engine.Run(ctx, jobID)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if res.CDOMKey == "" || res.ArtifactID == "" {
		t.Fatalf("result=%+v", res)
	}
	job, _ := jobs.Get(ctx, jobID)
	if job.Status != domain.JobCompleted || job.Progress != 100 {
		t.Fatalf("job=%+v", job)
	}
	rc, err := objects.Get(ctx, res.CDOMKey)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := io.ReadAll(rc)
	_ = rc.Close()
	doc, err := cdom.UnmarshalDocument(raw)
	if err != nil {
		t.Fatalf("cdom: %v raw=%s", err, raw)
	}
	if doc.DocumentID != docID {
		t.Fatalf("cdom id=%s", doc.DocumentID)
	}
	stage, prog, ok, _ := progress.GetProgress(ctx, jobID)
	if !ok || stage != "completed" || prog != 100 {
		t.Fatalf("progress stage=%s prog=%d ok=%v", stage, prog, ok)
	}
}

func TestEngineScannedUsesOCR(t *testing.T) {
	ctx := context.Background()
	docs := memory.NewDocumentRepo()
	jobs := memory.NewJobRepo()
	artifacts := memory.NewArtifactRepo()
	objects := memory.NewObjectStore()

	pdf := []byte("%PDF-1.4\n1 0 obj<< /Type /Page >>endobj\n")
	docID := "doc-scan"
	jobID := "job-scan"
	key := "documents/" + docID + "/original.pdf"
	_ = objects.Put(ctx, key, bytes.NewReader(pdf), int64(len(pdf)), "application/pdf")
	now := time.Now().UTC()
	_ = docs.Create(ctx, &domain.Document{
		ID: docID, Filename: "scan.pdf", ContentType: "application/pdf",
		StorageKey: key, Status: domain.DocumentQueued, CreatedAt: now, UpdatedAt: now,
	})
	_ = jobs.Create(ctx, &domain.Job{
		ID: jobID, DocumentID: docID, Status: domain.JobQueued, Stage: "queued", CreatedAt: now, UpdatedAt: now,
	})

	engine := &orchestrator.Engine{
		Documents: docs, Jobs: jobs, Artifacts: artifacts, Objects: objects, Progress: memory.NewProgress(),
		OCR: stubocr.New(),
	}
	res, err := engine.Run(ctx, jobID)
	if err != nil {
		t.Fatal(err)
	}
	if res.Kind != orchestrator.KindScanned {
		t.Fatalf("kind=%s", res.Kind)
	}
	rc, _ := objects.Get(ctx, res.CDOMKey)
	raw, _ := io.ReadAll(rc)
	_ = rc.Close()
	var payload map[string]any
	_ = json.Unmarshal(raw, &payload)
	proc := payload["processing"].(map[string]any)
	if proc["provider_name"] != "stub-ocr" {
		t.Fatalf("processing=%v", proc)
	}
}

func TestNormalizeToCDOMValid(t *testing.T) {
	doc := &domain.Document{ID: "d1", Filename: "a.pdf", ContentType: "application/pdf"}
	cdomDoc := orchestrator.NormalizeToCDOM(doc, orchestrator.KindDigital, nil, nil)
	cdomDoc.Pages = []cdom.Page{{PageNumber: 1, Width: 10, Height: 10, Blocks: []cdom.Block{}}}
	cdomDoc.Source.PageCount = 1
	if err := cdomDoc.Validate(); err != nil {
		t.Fatal(err)
	}
}
