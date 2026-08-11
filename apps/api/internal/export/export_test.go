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

func TestDOCXExporterProducesOOXMLZip(t *testing.T) {
	doc := sampleDoc()
	doc.Pages[0].Blocks = append(doc.Pages[0].Blocks,
		cdom.Block{
			ID: "list", Type: cdom.BlockList, BBox: cdom.BBox{0, 0, 1, 1}, ReadingOrder: 3, Confidence: 1,
			Attributes: map[string]any{"ordered": false},
			Children: []cdom.Block{
				{ID: "li1", Type: cdom.BlockListItem, BBox: cdom.BBox{0, 0, 1, 1}, ReadingOrder: 4, Confidence: 1, Text: "One"},
				{ID: "li2", Type: cdom.BlockListItem, BBox: cdom.BBox{0, 0, 1, 1}, ReadingOrder: 5, Confidence: 1, Text: "Two"},
			},
		},
		cdom.Block{
			ID: "tbl", Type: cdom.BlockTable, BBox: cdom.BBox{0, 0, 1, 1}, ReadingOrder: 6, Confidence: 1,
			Children: []cdom.Block{{
				ID: "r1", Type: cdom.BlockTableRow, BBox: cdom.BBox{0, 0, 1, 1}, ReadingOrder: 7, Confidence: 1,
				Children: []cdom.Block{
					{ID: "c1", Type: cdom.BlockTableCell, BBox: cdom.BBox{0, 0, 1, 1}, ReadingOrder: 8, Confidence: 1, Text: "A"},
					{ID: "c2", Type: cdom.BlockTableCell, BBox: cdom.BBox{0, 0, 1, 1}, ReadingOrder: 9, Confidence: 1, Text: "B"},
				},
			}},
		},
	)
	art, err := export.NewDOCX().Export(context.Background(), doc)
	if err != nil {
		t.Fatal(err)
	}
	if art.Format != "docx" {
		t.Fatalf("format=%s", art.Format)
	}
	if len(art.Body) < 100 || art.Body[0] != 'P' || art.Body[1] != 'K' {
		t.Fatalf("expected zip magic, got len=%d prefix=%v", len(art.Body), art.Body[:min(4, len(art.Body))])
	}
}
