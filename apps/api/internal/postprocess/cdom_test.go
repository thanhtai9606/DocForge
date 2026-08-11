package postprocess_test

import (
	"testing"

	"github.com/thanhtai9606/DocForge/apps/api/internal/postprocess"
	"github.com/thanhtai9606/DocForge/packages/cdom"
)

func TestCleanDropsEmptyBlocks(t *testing.T) {
	doc := &cdom.Document{
		SchemaVersion: cdom.SchemaVersion,
		DocumentID:    "d1",
		Source:        cdom.Source{Filename: "a.pdf", MimeType: "application/pdf", PageCount: 1},
		Metadata:      map[string]any{},
		Processing:    map[string]any{},
		Pages: []cdom.Page{{
			PageNumber: 1, Width: 100, Height: 100,
			Blocks: []cdom.Block{
				{ID: "p1", Type: cdom.BlockParagraph, BBox: cdom.BBox{0, 0, 1, 1}, Confidence: 1, Text: " keep "},
				{ID: "p2", Type: cdom.BlockParagraph, BBox: cdom.BBox{0, 0, 1, 1}, Confidence: 1, Text: "   "},
				{ID: "list", Type: cdom.BlockList, BBox: cdom.BBox{0, 0, 1, 1}, Confidence: 1, Children: nil},
			},
		}},
	}
	postprocess.Clean(doc)
	if len(doc.Pages[0].Blocks) != 1 {
		t.Fatalf("blocks=%+v", doc.Pages[0].Blocks)
	}
	if doc.Pages[0].Blocks[0].Text != "keep" {
		t.Fatalf("text=%q", doc.Pages[0].Blocks[0].Text)
	}
	if doc.Processing["postprocess_provider"] != postprocess.Name {
		t.Fatalf("processing=%v", doc.Processing)
	}
}
