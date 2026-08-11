package orchestrator_test

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/thanhtai9606/DocForge/apps/api/internal/domain"
	"github.com/thanhtai9606/DocForge/apps/api/internal/export"
	"github.com/thanhtai9606/DocForge/apps/api/internal/infrastructure/memory"
	"github.com/thanhtai9606/DocForge/apps/api/internal/layout"
	"github.com/thanhtai9606/DocForge/apps/api/internal/orchestrator"
	"github.com/thanhtai9606/DocForge/apps/api/internal/providers/stubocr"
	"github.com/thanhtai9606/DocForge/packages/cdom"
)

func buildPDF(pageContent string) []byte {
	objs := make([]string, 6)
	objs[1] = "1 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n"
	objs[2] = "2 0 obj\n<< /Type /Pages /Kids [3 0 R] /Count 1 >>\nendobj\n"
	objs[3] = "3 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Contents 4 0 R /Resources << /Font << /F1 5 0 R >> >> >>\nendobj\n"
	stream := "BT /F1 24 Tf 72 720 Td (" + pageContent + ") Tj ET\n"
	objs[4] = fmt.Sprintf("4 0 obj\n<< /Length %d >>\nstream\n%sengstream\nendobj\n", len(stream), stream)
	objs[4] = fmt.Sprintf("4 0 obj\n<< /Length %d >>\nstream\n%sendstream\nendobj\n", len(stream), stream)
	objs[5] = "5 0 obj\n<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>\nendobj\n"

	var body bytes.Buffer
	body.WriteString("%PDF-1.4\n%\xe2\xe3\xcf\xd3\n")
	offsets := make([]int, 6)
	for i := 1; i <= 5; i++ {
		offsets[i] = body.Len()
		body.WriteString(objs[i])
	}
	xrefPos := body.Len()
	body.WriteString("xref\n0 6\n")
	body.WriteString("0000000000 65535 f \n")
	for i := 1; i <= 5; i++ {
		body.WriteString(fmt.Sprintf("%010d 00000 n \n", offsets[i]))
	}
	body.WriteString("trailer\n<< /Size 6 /Root 1 0 R >>\nstartxref\n")
	body.WriteString(fmt.Sprintf("%d\n%%%%EOF\n", xrefPos))
	return body.Bytes()
}

func buildEmptyContentPDF() []byte {
	objs := make([]string, 6)
	objs[1] = "1 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n"
	objs[2] = "2 0 obj\n<< /Type /Pages /Kids [3 0 R] /Count 1 >>\nendobj\n"
	objs[3] = "3 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Contents 4 0 R >>\nendobj\n"
	stream := ""
	objs[4] = fmt.Sprintf("4 0 obj\n<< /Length %d >>\nstream\n%sendstream\nendobj\n", len(stream), stream)
	objs[5] = "5 0 obj\n<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>\nendobj\n"
	var body bytes.Buffer
	body.WriteString("%PDF-1.4\n%\xe2\xe3\xcf\xd3\n")
	offsets := make([]int, 6)
	for i := 1; i <= 5; i++ {
		offsets[i] = body.Len()
		body.WriteString(objs[i])
	}
	xrefPos := body.Len()
	body.WriteString("xref\n0 6\n0000000000 65535 f \n")
	for i := 1; i <= 5; i++ {
		body.WriteString(fmt.Sprintf("%010d 00000 n \n", offsets[i]))
	}
	body.WriteString("trailer\n<< /Size 6 /Root 1 0 R >>\nstartxref\n")
	body.WriteString(fmt.Sprintf("%d\n%%%%EOF\n", xrefPos))
	return body.Bytes()
}

func newEngine() (*orchestrator.Engine, *memory.DocumentRepo, *memory.JobRepo, *memory.ArtifactRepo, *memory.ObjectStore) {
	docs := memory.NewDocumentRepo()
	jobs := memory.NewJobRepo()
	arts := memory.NewArtifactRepo()
	objs := memory.NewObjectStore()
	eng := &orchestrator.Engine{
		Documents: docs, Jobs: jobs, Artifacts: arts, Objects: objs,
		Progress: memory.NewProgress(), OCR: stubocr.New(), Layout: layout.NewGeometric(),
		Exporters:        []export.Exporter{export.NewJSON(), export.NewMarkdown()},
		MaxParallelPages: 2,
	}
	return eng, docs, jobs, arts, objs
}

func seed(t *testing.T, docs *memory.DocumentRepo, jobs *memory.JobRepo, objs *memory.ObjectStore, docID, jobID string, pdf []byte) {
	t.Helper()
	ctx := context.Background()
	key := "documents/" + docID + "/original.pdf"
	if err := objs.Put(ctx, key, bytes.NewReader(pdf), int64(len(pdf)), "application/pdf"); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	_ = docs.Create(ctx, &domain.Document{
		ID: docID, Filename: docID + ".pdf", ContentType: "application/pdf",
		StorageKey: key, Status: domain.DocumentQueued, OutputFormats: []string{"markdown", "json"},
		CreatedAt: now, UpdatedAt: now,
	})
	_ = jobs.Create(ctx, &domain.Job{
		ID: jobID, DocumentID: docID, Status: domain.JobQueued, Stage: "queued", CreatedAt: now, UpdatedAt: now,
	})
}

func TestEngineDigitalProducesExports(t *testing.T) {
	eng, docs, jobs, arts, objs := newEngine()
	pdf := buildPDF("Production Pipeline Heading Hello digital paragraph content for extraction")
	seed(t, docs, jobs, objs, "doc-d", "job-d", pdf)
	res, err := eng.Run(context.Background(), "job-d")
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	job, _ := jobs.Get(context.Background(), "job-d")
	if job.Status != domain.JobCompleted {
		t.Fatalf("status=%s err=%s/%s", job.Status, job.ErrorCode, job.ErrorMsg)
	}
	if len(res.ArtifactIDs) < 2 {
		t.Fatalf("artifacts=%v", res.ArtifactIDs)
	}
	list, _ := arts.ListByDocument(context.Background(), "doc-d")
	var hasMD bool
	for _, a := range list {
		if a.Format == "markdown" {
			hasMD = true
			rc, err := objs.Get(context.Background(), a.StorageKey)
			if err != nil {
				t.Fatal(err)
			}
			buf := new(bytes.Buffer)
			_, _ = buf.ReadFrom(rc)
			_ = rc.Close()
			if buf.Len() == 0 {
				t.Fatal("empty markdown")
			}
		}
	}
	if !hasMD {
		t.Fatalf("missing markdown export: %+v", list)
	}
}

func TestEngineScannedUsesOCR(t *testing.T) {
	eng, docs, jobs, arts, objs := newEngine()
	seed(t, docs, jobs, objs, "doc-s", "job-s", buildEmptyContentPDF())
	res, err := eng.Run(context.Background(), "job-s")
	if err != nil {
		t.Fatal(err)
	}
	if res.Kind != orchestrator.KindScanned {
		t.Fatalf("kind=%s", res.Kind)
	}
	job, _ := jobs.Get(context.Background(), "job-s")
	if job.Status != domain.JobCompleted {
		t.Fatalf("job=%+v", job)
	}
	list, _ := arts.ListByDocument(context.Background(), "doc-s")
	if len(list) < 2 {
		t.Fatalf("expected cdom+exports, got %d", len(list))
	}
}

func TestMarkdownExporterTableAndHeading(t *testing.T) {
	doc := &cdom.Document{
		SchemaVersion: cdom.SchemaVersion,
		DocumentID:    "d",
		Source:        cdom.Source{Filename: "a.pdf", MimeType: "application/pdf", PageCount: 1},
		Metadata:      map[string]any{},
		Processing:    map[string]any{},
		Pages: []cdom.Page{{
			PageNumber: 1, Width: 100, Height: 100,
			Blocks: []cdom.Block{
				{ID: "h", Type: cdom.BlockHeading, BBox: cdom.BBox{0, 80, 50, 100}, ReadingOrder: 1, Confidence: 1, Text: "Title"},
				{ID: "p", Type: cdom.BlockParagraph, BBox: cdom.BBox{0, 40, 50, 60}, ReadingOrder: 2, Confidence: 1, Text: "Body"},
			},
		}},
	}
	art, err := export.NewMarkdown().Export(context.Background(), doc)
	if err != nil {
		t.Fatal(err)
	}
	s := string(art.Body)
	if !strings.Contains(s, "# Title") || !strings.Contains(s, "Body") {
		t.Fatalf("markdown=%q", s)
	}
}
