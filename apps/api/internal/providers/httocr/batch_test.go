package httocr_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/thanhtai9606/DocForge/apps/api/internal/orchestrator"
	"github.com/thanhtai9606/DocForge/apps/api/internal/providers/httocr"
)

func TestProcessBatch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/ocr/batch" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"pages": []map[string]any{
				{"page_number": 1, "text": "one", "confidence": 0.9, "language": "eng"},
				{"page_number": 2, "text": "two", "confidence": 0.8, "language": "eng"},
			},
			"provider": "docforge-ocr",
			"version":  "5.3.0",
		})
	}))
	defer srv.Close()

	p := httocr.New(srv.URL, 0)
	out, err := p.ProcessBatch(context.Background(), orchestrator.OCRBatchRequest{
		DocumentID:  "doc-1",
		PageNumbers: []int{1, 2},
		Language:    []string{"en"},
		Payload:     []byte("%PDF-fake"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if out[1].Text != "one" || out[2].Text != "two" {
		t.Fatalf("unexpected batch output: %+v", out)
	}
}
