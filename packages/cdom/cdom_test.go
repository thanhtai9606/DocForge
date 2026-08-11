package cdom_test

import (
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

func TestImageRequiresAssetID(t *testing.T) {
	doc := validDoc()
	doc.Pages[0].Blocks[0].Type = cdom.BlockImage
	doc.Pages[0].Blocks[0].Attributes = nil
	if err := doc.Validate(); err == nil {
		t.Fatal("expected image asset_id error")
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
