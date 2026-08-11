package export_test

import (
	"context"
	"strings"
	"testing"

	"github.com/thanhtai9606/DocForge/apps/api/internal/export"
	"github.com/thanhtai9606/DocForge/packages/cdom"
)

func sampleDoc() *cdom.Document {
	return &cdom.Document{
		SchemaVersion: cdom.SchemaVersion,
		DocumentID:    "d1",
		Source:        cdom.Source{Filename: "a.pdf", MimeType: "application/pdf", PageCount: 1},
		Metadata:      map[string]any{},
		Processing:    map[string]any{},
		Pages: []cdom.Page{{
			PageNumber: 1, Width: 100, Height: 100,
			Blocks: []cdom.Block{
				{ID: "h", Type: cdom.BlockHeading, BBox: cdom.BBox{0, 0, 1, 1}, ReadingOrder: 1, Confidence: 1, Text: "Hello"},
				{ID: "p", Type: cdom.BlockParagraph, BBox: cdom.BBox{0, 0, 1, 1}, ReadingOrder: 2, Confidence: 1, Text: "World"},
			},
		}},
	}
}

func TestJSONAndMarkdownExporters(t *testing.T) {
	doc := sampleDoc()
	js, err := export.NewJSON().Export(context.Background(), doc)
	if err != nil || !strings.Contains(string(js.Body), `"document_id": "d1"`) {
		t.Fatalf("json=%v err=%v", string(js.Body), err)
	}
	md, err := export.NewMarkdown().Export(context.Background(), doc)
	if err != nil || !strings.Contains(string(md.Body), "# Hello") {
		t.Fatalf("md=%v err=%v", string(md.Body), err)
	}
}
