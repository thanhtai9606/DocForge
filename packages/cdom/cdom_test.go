package cdom_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/thanhtai9606/DocForge/packages/cdom"
)

func validDoc() *cdom.Document {
	return &cdom.Document{
		SchemaVersion: cdom.SchemaVersion,
		DocumentID:    "doc-1",
		Source: cdom.Source{
			Filename:  "sample.pdf",
			MimeType:  "application/pdf",
			PageCount: 1,
		},
		Metadata:   map[string]any{},
		Processing: map[string]any{},
		Pages: []cdom.Page{
			{
				PageNumber: 1,
				Width:      100,
				Height:     200,
				Language:   "vi",
				Blocks: []cdom.Block{
					{
						ID:           "block-1",
						Type:         cdom.BlockParagraph,
						BBox:         cdom.BBox{10, 10, 90, 40},
						ReadingOrder: 1,
						Confidence:   0.9,
						Text:         "hello",
					},
				},
			},
		},
	}
}

func TestDocumentValidateOK(t *testing.T) {
	if err := validDoc().Validate(); err != nil {
		t.Fatalf("expected valid document, got %v", err)
	}
}

func TestDocumentRejectsBadSchema(t *testing.T) {
	doc := validDoc()
	doc.SchemaVersion = "9.9"
	if err := doc.Validate(); err == nil {
		t.Fatal("expected schema_version error")
	}
}

func TestDocumentRejectsNilAndMissingFields(t *testing.T) {
	var nilDoc *cdom.Document
	if err := nilDoc.Validate(); err == nil {
		t.Fatal("expected nil document error")
	}
	doc := validDoc()
	doc.DocumentID = ""
	if err := doc.Validate(); err == nil || !strings.Contains(err.Error(), "document_id") {
		t.Fatalf("expected document_id error, got %v", err)
	}
	doc = validDoc()
	doc.Pages = nil
	if err := doc.Validate(); err == nil {
		t.Fatal("expected pages present error")
	}
}

func TestPageValidation(t *testing.T) {
	doc := validDoc()
	doc.Pages[0].PageNumber = 0
	if err := doc.Validate(); err == nil {
		t.Fatal("expected page_number error")
	}
	doc = validDoc()
	doc.Pages[0].Width = 0
	if err := doc.Validate(); err == nil {
		t.Fatal("expected width/height error")
	}
}

func TestBlockValidation(t *testing.T) {
	doc := validDoc()
	doc.Pages[0].Blocks[0].Type = "nope"
	if err := doc.Validate(); err == nil {
		t.Fatal("expected unsupported type")
	}
	doc = validDoc()
	doc.Pages[0].Blocks[0].BBox = cdom.BBox{10, 10, 5, 40}
	if err := doc.Validate(); err == nil {
		t.Fatal("expected invalid bbox")
	}
	doc = validDoc()
	doc.Pages[0].Blocks[0].Confidence = 1.5
	if err := doc.Validate(); err == nil {
		t.Fatal("expected confidence error")
	}
}

func TestImageRequiresAssetID(t *testing.T) {
	doc := validDoc()
	doc.Pages[0].Blocks[0].Type = cdom.BlockImage
	doc.Pages[0].Blocks[0].Attributes = nil
	if err := doc.Validate(); err == nil {
		t.Fatal("expected image asset_id error")
	}
	doc.Pages[0].Blocks[0].Attributes = map[string]any{"asset_id": "asset-1", "alt": "diagram"}
	if err := doc.Validate(); err != nil {
		t.Fatalf("valid image failed: %v", err)
	}
}

func TestNestedChildrenValidation(t *testing.T) {
	doc := validDoc()
	doc.Pages[0].Blocks[0].Type = cdom.BlockTable
	doc.Pages[0].Blocks[0].Children = []cdom.Block{{
		ID:         "row-1",
		Type:       cdom.BlockTableRow,
		BBox:       cdom.BBox{10, 10, 90, 20},
		Confidence: 1,
		Children: []cdom.Block{{
			ID:         "cell-1",
			Type:       cdom.BlockTableCell,
			BBox:       cdom.BBox{10, 10, 40, 20},
			Confidence: 1,
			Text:       "A",
		}},
	}}
	if err := doc.Validate(); err != nil {
		t.Fatalf("nested table invalid: %v", err)
	}
	doc.Pages[0].Blocks[0].Children[0].Children[0].ID = ""
	if err := doc.Validate(); err == nil {
		t.Fatal("expected nested child validation error")
	}
}

func TestUnmarshalDocument(t *testing.T) {
	raw := []byte(`{
		"schema_version":"1.0",
		"document_id":"doc-1",
		"source":{"filename":"a.pdf","mime_type":"application/pdf","page_count":1},
		"metadata":{},
		"processing":{},
		"pages":[{"page_number":1,"width":10,"height":10,"blocks":[{"id":"b1","type":"paragraph","bbox":[0,0,1,1],"reading_order":0,"confidence":1}]}]
	}`)
	doc, err := cdom.UnmarshalDocument(raw)
	if err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if doc.DocumentID != "doc-1" {
		t.Fatalf("unexpected id %s", doc.DocumentID)
	}
}

func TestUnmarshalRejectsInvalid(t *testing.T) {
	_, err := cdom.UnmarshalDocument([]byte(`{"schema_version":"1.0"}`))
	if err == nil {
		t.Fatal("expected validation error")
	}
	_, err = cdom.UnmarshalDocument([]byte(`{`))
	if err == nil {
		t.Fatal("expected json error")
	}
}

func TestMarshalJSONRoundTrip(t *testing.T) {
	doc := validDoc()
	raw, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	got, err := cdom.UnmarshalDocument(raw)
	if err != nil {
		t.Fatal(err)
	}
	if got.Pages[0].Blocks[0].Text != "hello" {
		t.Fatalf("text=%q", got.Pages[0].Blocks[0].Text)
	}
}
