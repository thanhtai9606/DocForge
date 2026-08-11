package pdfx_test

import (
	"testing"

	"github.com/thanhtai9606/DocForge/apps/api/internal/pdfx"
)

func TestExtractInvalidMagic(t *testing.T) {
	_, err := pdfx.Extract([]byte("not-pdf"), pdfx.ExtractOptions{MaxParallelPages: 2})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestClassify(t *testing.T) {
	doc := &pdfx.DocumentText{PageCount: 2, Pages: []pdfx.PageText{
		{PageNumber: 1, CharCount: 100},
		{PageNumber: 2, CharCount: 0},
	}}
	kind, d, s := pdfx.Classify(doc, 40)
	if kind != "mixed" || d != 1 || s != 1 {
		t.Fatalf("kind=%s d=%d s=%d", kind, d, s)
	}
}
