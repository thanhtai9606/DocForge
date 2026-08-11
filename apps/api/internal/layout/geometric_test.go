package layout_test

import (
	"context"
	"testing"

	"github.com/thanhtai9606/DocForge/apps/api/internal/layout"
	"github.com/thanhtai9606/DocForge/packages/cdom"
)

func TestGeometricReconstructsListsAndTables(t *testing.T) {
	doc := &cdom.Document{
		SchemaVersion: cdom.SchemaVersion,
		DocumentID:    "d1",
		Source:        cdom.Source{Filename: "a.pdf", MimeType: "application/pdf", PageCount: 1},
		Metadata:      map[string]any{},
		Processing:    map[string]any{},
		Pages: []cdom.Page{{
			PageNumber: 1, Width: 612, Height: 792,
			Blocks: []cdom.Block{
				{ID: "h", Type: cdom.BlockParagraph, BBox: cdom.BBox{72, 700, 300, 720}, Confidence: 0.9, Text: "Overview"},
				{ID: "l1", Type: cdom.BlockParagraph, BBox: cdom.BBox{72, 660, 300, 680}, Confidence: 0.9, Text: "- Alpha"},
				{ID: "l2", Type: cdom.BlockParagraph, BBox: cdom.BBox{72, 640, 300, 660}, Confidence: 0.9, Text: "- Beta"},
				{ID: "t1", Type: cdom.BlockParagraph, BBox: cdom.BBox{72, 600, 400, 620}, Confidence: 0.9, Text: "| Name | Qty |"},
				{ID: "t2", Type: cdom.BlockParagraph, BBox: cdom.BBox{72, 580, 400, 600}, Confidence: 0.9, Text: "| --- | --- |"},
				{ID: "t3", Type: cdom.BlockParagraph, BBox: cdom.BBox{72, 560, 400, 580}, Confidence: 0.9, Text: "| Widget | 3 |"},
			},
		}},
	}
	p := layout.NewGeometric()
	if err := p.Analyze(context.Background(), doc); err != nil {
		t.Fatal(err)
	}
	blocks := doc.Pages[0].Blocks
	if len(blocks) < 3 {
		t.Fatalf("blocks=%+v", blocks)
	}
	if blocks[0].Type != cdom.BlockHeading {
		t.Fatalf("want heading, got %s (%q)", blocks[0].Type, blocks[0].Text)
	}
	var sawList, sawTable bool
	for _, b := range blocks {
		switch b.Type {
		case cdom.BlockList:
			sawList = true
			if len(b.Children) != 2 {
				t.Fatalf("list children=%d", len(b.Children))
			}
			if b.Children[0].Text != "Alpha" || b.Children[1].Text != "Beta" {
				t.Fatalf("list items=%+v", b.Children)
			}
		case cdom.BlockTable:
			sawTable = true
			if len(b.Children) != 2 {
				t.Fatalf("table rows=%d", len(b.Children))
			}
			if b.Children[0].Children[0].Text != "Name" {
				t.Fatalf("header cell=%q", b.Children[0].Children[0].Text)
			}
		}
	}
	if !sawList || !sawTable {
		t.Fatalf("list=%v table=%v blocks=%+v", sawList, sawTable, blocks)
	}
	if doc.Processing["layout_provider_version"] != p.Version() {
		t.Fatalf("processing=%v", doc.Processing)
	}
}

func TestGeometricReconstructsSpaceAndColumnTables(t *testing.T) {
	doc := &cdom.Document{
		SchemaVersion: cdom.SchemaVersion,
		DocumentID:    "d2",
		Source:        cdom.Source{Filename: "t.pdf", MimeType: "application/pdf", PageCount: 1},
		Metadata:      map[string]any{},
		Processing:    map[string]any{},
		Pages: []cdom.Page{{
			PageNumber: 1, Width: 612, Height: 792,
			Blocks: []cdom.Block{
				{ID: "s1", Type: cdom.BlockParagraph, BBox: cdom.BBox{72, 700, 400, 720}, Confidence: 0.9, Text: "Name    Qty"},
				{ID: "s2", Type: cdom.BlockParagraph, BBox: cdom.BBox{72, 680, 400, 700}, Confidence: 0.9, Text: "Widget  3"},
				{ID: "c1", Type: cdom.BlockParagraph, BBox: cdom.BBox{72, 600, 150, 620}, Confidence: 0.9, Text: "A"},
				{ID: "c2", Type: cdom.BlockParagraph, BBox: cdom.BBox{220, 600, 300, 620}, Confidence: 0.9, Text: "B"},
				{ID: "c3", Type: cdom.BlockParagraph, BBox: cdom.BBox{72, 560, 150, 580}, Confidence: 0.9, Text: "C"},
				{ID: "c4", Type: cdom.BlockParagraph, BBox: cdom.BBox{220, 560, 300, 580}, Confidence: 0.9, Text: "D"},
			},
		}},
	}
	if err := layout.NewGeometric().Analyze(context.Background(), doc); err != nil {
		t.Fatal(err)
	}
	var tables int
	for _, b := range doc.Pages[0].Blocks {
		if b.Type == cdom.BlockTable {
			tables++
			if len(b.Children) < 2 {
				t.Fatalf("table rows=%d", len(b.Children))
			}
		}
	}
	if tables < 2 {
		t.Fatalf("expected space+geometric tables, got %d blocks=%+v", tables, doc.Pages[0].Blocks)
	}
}
