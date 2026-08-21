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
	"github.com/thanhtai9606/DocForge/apps/api/internal/auth"
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

func newQuotaHarness(t *testing.T, bypass bool, anonLimit, authLimit int) *harness {
	t.Helper()
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
	handler := api.NewServer(svc, api.Options{
		Auth:            auth.New("test-secret", bypass, "http://localhost:5173", "http://localhost:8080", auth.ProviderConfig{}, auth.ProviderConfig{}),
		Quota:           memory.NewQuotaStore(),
		AnonUploadLimit: anonLimit,
		AuthUploadLimit: authLimit,
	})
	return &harness{handler: handler, svc: svc, artifacts: artifacts, objects: objects}
}

func uploadPDFWithCookie(t *testing.T, h http.Handler, guestID, filename string, content []byte) *httptest.ResponseRecorder {
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
	_ = w.WriteField("output_formats", "markdown")
	_ = w.Close()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/documents", &body)
	req.Header.Set("Content-Type", w.FormDataContentType())
	if guestID != "" {
		req.AddCookie(&http.Cookie{Name: auth.GuestCookieName, Value: guestID})
	}
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	return rr
}

func TestAnonymousUploadQuota(t *testing.T) {
	h := newQuotaHarness(t, false, 3, 10)
	pdf := []byte("%PDF-1.4\n%test")
	for i := 0; i < 3; i++ {
		rr := uploadPDFWithCookie(t, h.handler, "guest-quota-test", "sample.pdf", pdf)
		if rr.Code != http.StatusAccepted {
			t.Fatalf("upload %d status=%d body=%s", i+1, rr.Code, rr.Body.String())
		}
	}
	rr := uploadPDFWithCookie(t, h.handler, "guest-quota-test", "sample.pdf", pdf)
	if rr.Code != http.StatusTooManyRequests {
		t.Fatalf("expected quota exceeded, status=%d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "QUOTA_EXCEEDED") {
		t.Fatalf("expected QUOTA_EXCEEDED body=%s", rr.Body.String())
	}
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
	if cd := rr.Header().Get("Content-Disposition"); !strings.Contains(cd, "sample.json") {
		t.Fatalf("content-disposition=%q", cd)
	}
	if rr.Body.String() != `{"ok":true}` {
		t.Fatalf("body=%q", rr.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/api/v1/artifacts/art-json", nil)
	rr = httptest.NewRecorder()
	h.handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK || !bytes.Contains(rr.Body.Bytes(), []byte(`"download_url"`)) {
		t.Fatalf("get artifact status=%d body=%s", rr.Code, rr.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/api/v1/documents/"+docID+"/artifacts?kind=export&format=json", nil)
	rr = httptest.NewRecorder()
	h.handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK || !bytes.Contains(rr.Body.Bytes(), []byte("art-json")) {
		t.Fatalf("filtered list status=%d body=%s", rr.Code, rr.Body.String())
	}
}

func TestSaveMarkdownAndJobEvents(t *testing.T) {
	h := newHarness()
	docID, jobID := uploadPDF(t, h.handler, "sample.pdf", []byte("%PDF-1.4"), "")
	key := "documents/" + docID + "/exports/output.md"
	if err := h.objects.Put(context.Background(), key, strings.NewReader("old"), 3, "text/markdown"); err != nil {
		t.Fatal(err)
	}
	if err := h.artifacts.Create(context.Background(), &domain.Artifact{
		ID: "md-1", DocumentID: docID, JobID: jobID, Kind: "export", Format: "markdown",
		StorageKey: key, SizeBytes: 3, CreatedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodPut, "/api/v1/documents/"+docID+"/markdown", strings.NewReader("# saved\n"))
	req.Header.Set("Content-Type", "text/markdown")
	rr := httptest.NewRecorder()
	h.handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK || !bytes.Contains(rr.Body.Bytes(), []byte("md-1")) {
		t.Fatalf("save status=%d body=%s", rr.Code, rr.Body.String())
	}

	req = httptest.NewRequest(http.MethodPost, "/api/v1/jobs/"+jobID+"/cancel", nil)
	rr = httptest.NewRecorder()
	h.handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("cancel status=%d body=%s", rr.Code, rr.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/api/v1/jobs/"+jobID+"/events", nil)
	rr = httptest.NewRecorder()
	h.handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("sse status=%d", rr.Code)
	}
	if ct := rr.Header().Get("Content-Type"); !strings.Contains(ct, "text/event-stream") {
		t.Fatalf("content-type=%q", ct)
	}
	if !bytes.Contains(rr.Body.Bytes(), []byte("event: progress")) {
		t.Fatalf("sse body=%s", rr.Body.String())
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
