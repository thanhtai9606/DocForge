package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/thanhtai9606/DocForge/apps/api/internal/api"
	"github.com/thanhtai9606/DocForge/apps/api/internal/application"
	"github.com/thanhtai9606/DocForge/apps/api/internal/domain"
	"github.com/thanhtai9606/DocForge/apps/api/internal/infrastructure/memory"
)

type harness struct {
	handler   http.Handler
	svc       *application.Service
	artifacts *memory.ArtifactRepo
	objects   *memory.ObjectStore
}

func newHarness() *harness {
	artifacts := memory.NewArtifactRepo()
	objects := memory.NewObjectStore()
	svc := &application.Service{
		Documents: memory.NewDocumentRepo(),
		Jobs:      memory.NewJobRepo(),
		Artifacts: artifacts,
		Objects:   objects,
		Queue:     memory.NewQueue(),
		Progress:  memory.NewProgress(),
		MaxBytes:  1 << 20,
		MaxPages:  100,
	}
	return &harness{
		handler:   api.NewServer(svc),
		svc:       svc,
		artifacts: artifacts,
		objects:   objects,
	}
}

func uploadPDF(t *testing.T, h http.Handler, filename string, content []byte, formats string) (docID, jobID string) {
	t.Helper()
	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	fw, err := w.CreateFormFile("file", filename)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fw.Write(content); err != nil {
		t.Fatal(err)
	}
	if formats != "" {
		_ = w.WriteField("output_formats", formats)
	}
	_ = w.Close()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/documents", &body)
	req.Header.Set("Content-Type", w.FormDataContentType())
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusAccepted {
		t.Fatalf("upload status=%d body=%s", rr.Code, rr.Body.String())
	}
	var upload map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &upload); err != nil {
		t.Fatal(err)
	}
	return upload["document_id"].(string), upload["job_id"].(string)
}

func TestUploadAndGetJob(t *testing.T) {
	h := newHarness()
	docID, jobID := uploadPDF(t, h.handler, "sample.pdf", []byte("%PDF-1.4\n%test"), "markdown,json")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/jobs/"+jobID, nil)
	rr := httptest.NewRecorder()
	h.handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("get job status=%d body=%s", rr.Code, rr.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/api/v1/documents/"+docID, nil)
	rr = httptest.NewRecorder()
	h.handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("get document status=%d body=%s", rr.Code, rr.Body.String())
	}

	req = httptest.NewRequest(http.MethodPost, "/api/v1/jobs/"+jobID+"/cancel", nil)
	rr = httptest.NewRecorder()
	h.handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("cancel status=%d body=%s", rr.Code, rr.Body.String())
	}
}

func TestRejectNonPDF(t *testing.T) {
	h := newHarness()
	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	fw, _ := w.CreateFormFile("file", "note.txt")
	_, _ = fw.Write([]byte("hello"))
	_ = w.Close()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/documents", &body)
	req.Header.Set("Content-Type", w.FormDataContentType())
	rr := httptest.NewRecorder()
	h.handler.ServeHTTP(rr, req)
	if rr.Code == http.StatusAccepted {
		t.Fatal("expected rejection")
	}
	b, _ := io.ReadAll(rr.Body)
	if !bytes.Contains(b, []byte("INVALID_PDF")) {
		t.Fatalf("expected INVALID_PDF, got %s", string(b))
	}
}

func TestHealthz(t *testing.T) {
	h := newHarness()
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rr := httptest.NewRecorder()
	h.handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d", rr.Code)
	}
}

func TestGetJobNotFound(t *testing.T) {
	h := newHarness()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/jobs/missing", nil)
	req.Header.Set("X-Request-ID", "req-missing")
	rr := httptest.NewRecorder()
	h.handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if rr.Header().Get("X-Request-ID") != "req-missing" {
		t.Fatalf("request id header=%q", rr.Header().Get("X-Request-ID"))
	}
	if !bytes.Contains(rr.Body.Bytes(), []byte("req-missing")) {
		t.Fatalf("body missing request_id: %s", rr.Body.String())
	}
}

func TestMissingFileField(t *testing.T) {
	h := newHarness()
	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	_ = w.WriteField("output_formats", "json")
	_ = w.Close()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/documents", &body)
	req.Header.Set("Content-Type", w.FormDataContentType())
	rr := httptest.NewRecorder()
	h.handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
}

func TestArtifactsListAndDownload(t *testing.T) {
	h := newHarness()
	docID, jobID := uploadPDF(t, h.handler, "sample.pdf", []byte("%PDF-1.4"), "")
	key := "artifacts/" + docID + "/out.json"
	if err := h.objects.Put(context.Background(), key, strings.NewReader(`{"ok":true}`), 11, "application/json"); err != nil {
		t.Fatal(err)
	}
	if err := h.artifacts.Create(context.Background(), &domain.Artifact{
		ID:         "art-json",
		DocumentID: docID,
		JobID:      jobID,
		Kind:       "export",
		Format:     "json",
		StorageKey: key,
		SizeBytes:  11,
		CreatedAt:  time.Now().UTC(),
	}); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/documents/"+docID+"/artifacts", nil)
	rr := httptest.NewRecorder()
	h.handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("list status=%d body=%s", rr.Code, rr.Body.String())
	}
	if !bytes.Contains(rr.Body.Bytes(), []byte("art-json")) {
		t.Fatalf("list body=%s", rr.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/api/v1/artifacts/art-json/download", nil)
	rr = httptest.NewRecorder()
	h.handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("download status=%d body=%s", rr.Code, rr.Body.String())
	}
	if ct := rr.Header().Get("Content-Type"); ct != "application/json" {
		t.Fatalf("content-type=%q", ct)
	}
	if rr.Body.String() != `{"ok":true}` {
		t.Fatalf("body=%q", rr.Body.String())
	}
}

func TestDownloadArtifactNotFound(t *testing.T) {
	h := newHarness()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/artifacts/nope/download", nil)
	rr := httptest.NewRecorder()
	h.handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("status=%d", rr.Code)
	}
}
