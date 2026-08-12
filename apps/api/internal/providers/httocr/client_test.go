package httocr_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/thanhtai9606/DocForge/apps/api/internal/orchestrator"
	"github.com/thanhtai9606/DocForge/apps/api/internal/providers/httocr"
)

func TestProcessSuccess(t *testing.T) {
	var gotPage int
	var gotB64 string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/ocr" {
			http.NotFound(w, r)
			return
		}
		raw, _ := io.ReadAll(r.Body)
		var in struct {
			PageNumber int    `json:"page_number"`
			PDF        string `json:"pdf_base64"`
		}
		_ = json.Unmarshal(raw, &in)
		gotPage = in.PageNumber
		gotB64 = in.PDF
		_ = json.NewEncoder(w).Encode(map[string]any{
			"text": "Xin chào", "confidence": 0.91, "language": "vie",
			"provider": "tesseract", "version": "5.3.0",
		})
	}))
	defer srv.Close()

	p := httocr.New(srv.URL, 5*time.Second)
	res, err := p.Process(context.Background(), orchestrator.OCRRequest{
		DocumentID: "d1", PageNumber: 2, Language: []string{"vi"}, Payload: []byte("%PDF-1.4"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if gotPage != 2 {
		t.Fatalf("page=%d", gotPage)
	}
	if decoded, _ := base64.StdEncoding.DecodeString(gotB64); string(decoded) != "%PDF-1.4" {
		t.Fatalf("payload=%q", gotB64)
	}
	if res.Text != "Xin chào" || res.Confidence != 0.91 || res.Language != "vie" {
		t.Fatalf("res=%+v", res)
	}
	if p.Name() != "tesseract" || p.Version() != "5.3.0" {
		t.Fatalf("identity=%s/%s", p.Name(), p.Version())
	}
}

func TestProcessSidecarError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "tesseract missing"})
	}))
	defer srv.Close()

	p := httocr.New(srv.URL, time.Second)
	_, err := p.Process(context.Background(), orchestrator.OCRRequest{PageNumber: 1, Payload: []byte("%PDF")})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestProcessRejectsInvalidPage(t *testing.T) {
	p := httocr.New("http://example.invalid", time.Second)
	_, err := p.Process(context.Background(), orchestrator.OCRRequest{PageNumber: 0, Payload: []byte("%PDF")})
	if err == nil {
		t.Fatal("expected invalid page")
	}
}
