package api_test

import (
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/thanhtai9606/DocForge/apps/api/internal/api"
	"github.com/thanhtai9606/DocForge/apps/api/internal/application"
	"github.com/thanhtai9606/DocForge/apps/api/internal/infrastructure/memory"
)

func newTestHandler() http.Handler {
	svc := &application.Service{
		Documents: memory.NewDocumentRepo(),
		Jobs:      memory.NewJobRepo(),
		Artifacts: memory.NewArtifactRepo(),
		Objects:   memory.NewObjectStore(),
		Queue:     memory.NewQueue(),
		Progress:  memory.NewProgress(),
		MaxBytes:  1 << 20,
		MaxPages:  100,
	}
	return api.NewServer(svc)
}

func TestUploadAndGetJob(t *testing.T) {
	h := newTestHandler()

	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	fw, err := w.CreateFormFile("file", "sample.pdf")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fw.Write([]byte("%PDF-1.4\n%test")); err != nil {
		t.Fatal(err)
	}
	_ = w.WriteField("output_formats", "markdown,json")
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
	jobID, _ := upload["job_id"].(string)
	docID, _ := upload["document_id"].(string)
	if jobID == "" || docID == "" {
		t.Fatalf("missing ids: %#v", upload)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/v1/jobs/"+jobID, nil)
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("get job status=%d body=%s", rr.Code, rr.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/api/v1/documents/"+docID, nil)
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("get document status=%d body=%s", rr.Code, rr.Body.String())
	}

	req = httptest.NewRequest(http.MethodPost, "/api/v1/jobs/"+jobID+"/cancel", nil)
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("cancel status=%d body=%s", rr.Code, rr.Body.String())
	}
}

func TestRejectNonPDF(t *testing.T) {
	h := newTestHandler()
	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	fw, _ := w.CreateFormFile("file", "note.txt")
	_, _ = fw.Write([]byte("hello"))
	_ = w.Close()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/documents", &body)
	req.Header.Set("Content-Type", w.FormDataContentType())
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code == http.StatusAccepted {
		t.Fatal("expected rejection")
	}
	b, _ := io.ReadAll(rr.Body)
	if !bytes.Contains(b, []byte("INVALID_PDF")) {
		t.Fatalf("expected INVALID_PDF, got %s", string(b))
	}
}
