package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/thanhtai9606/DocForge/apps/api/internal/application"
	"github.com/thanhtai9606/DocForge/apps/api/internal/artifacts"
	"github.com/thanhtai9606/DocForge/apps/api/internal/domain"
	"github.com/thanhtai9606/DocForge/apps/api/internal/logging"
)

// Server exposes REST handlers.
type Server struct {
	Service *application.Service
}

func NewServer(svc *application.Service) http.Handler {
	s := &Server{Service: svc}
	r := chi.NewRouter()
	r.Use(requestIDMiddleware)
	r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	r.Route("/api/v1", func(r chi.Router) {
		r.Post("/documents", s.createDocument)
		r.Get("/documents/{documentID}", s.getDocument)
		r.Get("/documents/{documentID}/artifacts", s.listArtifacts)
		r.Get("/jobs/{jobID}", s.getJob)
		r.Post("/jobs/{jobID}/cancel", s.cancelJob)
		r.Get("/artifacts/{artifactID}", s.getArtifact)
		r.Get("/artifacts/{artifactID}/download", s.downloadArtifact)
	})
	return r
}

func (s *Server) createDocument(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(s.Service.MaxBytes + (1 << 20)); err != nil {
		writeError(w, r, domain.NewAppError(domain.CodeInvalidPDF, "invalid multipart form", false))
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		writeError(w, r, domain.NewAppError(domain.CodeInvalidPDF, "file field is required", false))
		return
	}
	defer file.Close()

	formats := r.MultipartForm.Value["output_formats"]
	if len(formats) == 1 && strings.Contains(formats[0], ",") {
		formats = splitCSV(formats[0])
	}

	result, err := s.Service.UploadDocument(r.Context(), application.UploadInput{
		Filename:      header.Filename,
		ContentType:   header.Header.Get("Content-Type"),
		SizeBytes:     header.Size,
		Body:          file,
		OutputFormats: formats,
	})
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{
		"document_id": result.DocumentID,
		"job_id":      result.JobID,
		"status":      result.Status,
	})
}

func (s *Server) getJob(w http.ResponseWriter, r *http.Request) {
	job, err := s.Service.GetJob(r.Context(), chi.URLParam(r, "jobID"))
	if err != nil {
		writeError(w, r, err)
		return
	}
	var jobErr any
	if job.ErrorCode != "" {
		jobErr = map[string]any{"code": job.ErrorCode, "message": job.ErrorMsg}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"job_id":      job.ID,
		"document_id": job.DocumentID,
		"status":      job.Status,
		"stage":       job.Stage,
		"progress":    job.Progress,
		"error":       jobErr,
	})
}

func (s *Server) getDocument(w http.ResponseWriter, r *http.Request) {
	doc, err := s.Service.GetDocument(r.Context(), chi.URLParam(r, "documentID"))
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"document_id":    doc.ID,
		"filename":       doc.Filename,
		"content_type":   doc.ContentType,
		"size_bytes":     doc.SizeBytes,
		"page_count":     doc.PageCount,
		"status":         doc.Status,
		"output_formats": doc.OutputFormats,
		"created_at":     doc.CreatedAt,
		"updated_at":     doc.UpdatedAt,
	})
}

func (s *Server) listArtifacts(w http.ResponseWriter, r *http.Request) {
	documentID := chi.URLParam(r, "documentID")
	doc, err := s.Service.GetDocument(r.Context(), documentID)
	if err != nil {
		writeError(w, r, err)
		return
	}
	list, err := s.Service.ListArtifacts(r.Context(), documentID)
	if err != nil {
		writeError(w, r, err)
		return
	}
	kindFilter := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("kind")))
	formatFilter := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("format")))
	items := make([]map[string]any, 0, len(list))
	for _, a := range list {
		if kindFilter != "" && strings.ToLower(a.Kind) != kindFilter {
			continue
		}
		if formatFilter != "" && strings.ToLower(a.Format) != formatFilter {
			continue
		}
		name := artifacts.DownloadName(doc, &a)
		items = append(items, map[string]any{
			"artifact_id":   a.ID,
			"document_id":   a.DocumentID,
			"job_id":        a.JobID,
			"kind":          a.Kind,
			"format":        a.Format,
			"filename":      name,
			"size_bytes":    a.SizeBytes,
			"download_url":  "/api/v1/artifacts/" + a.ID + "/download",
			"created_at":    a.CreatedAt,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"artifacts": items})
}

func (s *Server) getArtifact(w http.ResponseWriter, r *http.Request) {
	artifact, err := s.Service.GetArtifact(r.Context(), chi.URLParam(r, "artifactID"))
	if err != nil {
		writeError(w, r, err)
		return
	}
	doc, err := s.Service.GetDocument(r.Context(), artifact.DocumentID)
	if err != nil {
		writeError(w, r, err)
		return
	}
	name := artifacts.DownloadName(doc, artifact)
	writeJSON(w, http.StatusOK, map[string]any{
		"artifact_id":  artifact.ID,
		"document_id":  artifact.DocumentID,
		"job_id":       artifact.JobID,
		"kind":         artifact.Kind,
		"format":       artifact.Format,
		"filename":     name,
		"size_bytes":   artifact.SizeBytes,
		"download_url": "/api/v1/artifacts/" + artifact.ID + "/download",
		"created_at":   artifact.CreatedAt,
	})
}

func (s *Server) downloadArtifact(w http.ResponseWriter, r *http.Request) {
	artifact, rc, err := s.Service.OpenArtifact(r.Context(), chi.URLParam(r, "artifactID"))
	if err != nil {
		writeError(w, r, err)
		return
	}
	defer rc.Close()
	doc, _ := s.Service.GetDocument(r.Context(), artifact.DocumentID)
	name := artifacts.DownloadName(doc, artifact)
	w.Header().Set("Content-Type", artifacts.ContentTypeForFormat(artifact.Format))
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, name))
	if artifact.SizeBytes > 0 {
		w.Header().Set("Content-Length", strconv.FormatInt(artifact.SizeBytes, 10))
	}
	w.WriteHeader(http.StatusOK)
	_, _ = io.Copy(w, rc)
}

func (s *Server) cancelJob(w http.ResponseWriter, r *http.Request) {
	job, err := s.Service.CancelJob(r.Context(), chi.URLParam(r, "jobID"))
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"job_id":      job.ID,
		"document_id": job.DocumentID,
		"status":      job.Status,
		"stage":       job.Stage,
		"progress":    job.Progress,
	})
}

func requestIDMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get("X-Request-ID")
		if id == "" {
			id = uuid.NewString()
		}
		ctx := logging.WithRequestID(r.Context(), id)
		w.Header().Set("X-Request-ID", id)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeError(w http.ResponseWriter, r *http.Request, err error) {
	appErr, ok := domain.AsAppError(err)
	if !ok {
		var ae *domain.AppError
		if errors.As(err, &ae) {
			appErr = ae
			ok = true
		}
	}
	if !ok {
		appErr = domain.NewAppError(domain.CodeInternal, "internal server error", true)
	}
	status := http.StatusBadRequest
	switch appErr.Code {
	case domain.CodeNotFound:
		status = http.StatusNotFound
	case domain.CodeFileTooLarge:
		status = http.StatusRequestEntityTooLarge
	case domain.CodeInternal, domain.CodeStorageError, domain.CodeOCRProviderError:
		status = http.StatusInternalServerError
	}
	writeJSON(w, status, map[string]any{
		"error": map[string]any{
			"code":       appErr.Code,
			"message":    appErr.Message,
			"retryable":  appErr.Retryable,
			"request_id": logging.RequestID(r.Context()),
		},
	})
}

func splitCSV(v string) []string {
	parts := strings.Split(v, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}
