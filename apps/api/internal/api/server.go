package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/thanhtai9606/DocForge/apps/api/internal/application"
	"github.com/thanhtai9606/DocForge/apps/api/internal/artifacts"
	"github.com/thanhtai9606/DocForge/apps/api/internal/auth"
	"github.com/thanhtai9606/DocForge/apps/api/internal/domain"
	"github.com/thanhtai9606/DocForge/apps/api/internal/infrastructure/memory"
	"github.com/thanhtai9606/DocForge/apps/api/internal/logging"
	"github.com/thanhtai9606/DocForge/apps/api/internal/metrics"
	"github.com/thanhtai9606/DocForge/apps/api/internal/middleware"
	"github.com/thanhtai9606/DocForge/apps/api/internal/quota"
)

// Server exposes REST handlers.
type Server struct {
	Service *application.Service
	Auth    *auth.Service
	Metrics *metrics.Registry
	Quota   quota.Store
	AnonUploadLimit int
	AuthUploadLimit int
}

// Options configures HTTP middleware and auth for Phase 4/5.
type Options struct {
	Auth            *auth.Service
	Metrics         *metrics.Registry
	Quota           quota.Store
	AnonUploadLimit int
	AuthUploadLimit int
	CORSOrigins     []string
	RequestTimeout  time.Duration
	RatePerMinute   int
	RateBurst       int
}

func NewServer(svc *application.Service, opts ...Options) http.Handler {
	var o Options
	if len(opts) > 0 {
		o = opts[0]
	}
	s := &Server{
		Service:         svc,
		Auth:            o.Auth,
		Metrics:         o.Metrics,
		Quota:           o.Quota,
		AnonUploadLimit: o.AnonUploadLimit,
		AuthUploadLimit: o.AuthUploadLimit,
	}
	if s.Quota == nil {
		s.Quota = memory.NewQuotaStore()
	}
	if s.AnonUploadLimit <= 0 {
		s.AnonUploadLimit = 3
	}
	if s.AuthUploadLimit <= 0 {
		s.AuthUploadLimit = 10
	}
	if s.Auth == nil {
		s.Auth = auth.New("", true, "http://localhost:5173", "http://localhost:8080", auth.ProviderConfig{}, auth.ProviderConfig{})
	}
	if s.Metrics == nil {
		s.Metrics = metrics.New()
	}
	if o.RequestTimeout <= 0 {
		o.RequestTimeout = 60 * time.Second
	}
	if o.RatePerMinute <= 0 {
		o.RatePerMinute = 180
	}
	if o.RateBurst <= 0 {
		o.RateBurst = 40
	}
	if len(o.CORSOrigins) == 0 {
		o.CORSOrigins = []string{"http://localhost:5173", "http://127.0.0.1:5173"}
	}

	limiter := middleware.NewRateLimiter(o.RatePerMinute, o.RateBurst, s.Metrics)
	r := chi.NewRouter()
	r.Use(middleware.SecurityHeaders)
	r.Use(middleware.CORS(o.CORSOrigins))
	r.Use(middleware.Metrics(s.Metrics))
	r.Use(requestIDMiddleware)
	r.Use(s.Auth.Middleware)
	r.Use(limiter.Middleware)
	r.Use(middleware.Timeout(o.RequestTimeout))

	r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	r.Get("/metrics", s.Metrics.Handler())

	r.Route("/api/v1", func(r chi.Router) {
		r.Get("/auth/providers", s.authProviders)
		r.Get("/auth/{provider}/login", s.authLogin)
		r.Get("/auth/{provider}/callback", s.authCallback)
		r.Get("/auth/me", s.me)
		r.Post("/auth/logout", s.logout)
		r.Get("/quota", s.uploadQuota)

		r.Get("/documents", s.listDocuments)
		r.Post("/documents", s.createDocument)
		r.Get("/documents/{documentID}", s.getDocument)
		r.Delete("/documents/{documentID}", s.deleteDocument)
		r.Get("/documents/{documentID}/artifacts", s.listArtifacts)
		r.Get("/documents/{documentID}/job", s.latestJob)
		r.Get("/documents/{documentID}/original", s.downloadOriginal)
		r.Put("/documents/{documentID}/markdown", s.saveMarkdown)
		r.Post("/documents/{documentID}/markdown/regenerate", s.regenerateMarkdown)

		r.Get("/jobs/{jobID}", s.getJob)
		r.Get("/jobs/{jobID}/events", s.jobEvents)
		r.Post("/jobs/{jobID}/cancel", s.cancelJob)
		r.Post("/jobs/{jobID}/retry", s.retryJob)

		r.Get("/artifacts/{artifactID}", s.getArtifact)
		r.Get("/artifacts/{artifactID}/download", s.downloadArtifact)
	})
	return r
}

func (s *Server) authProviders(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"providers": s.Auth.EnabledProviders(),
		"bypass":    s.Auth.Bypass,
	})
}

func (s *Server) authLogin(w http.ResponseWriter, r *http.Request) {
	provider := chi.URLParam(r, "provider")
	url, err := s.Auth.AuthCodeURL(provider)
	if err != nil {
		writeError(w, r, err)
		return
	}
	http.Redirect(w, r, url, http.StatusFound)
}

func (s *Server) authCallback(w http.ResponseWriter, r *http.Request) {
	provider := chi.URLParam(r, "provider")
	code := r.URL.Query().Get("code")
	state := r.URL.Query().Get("state")
	if code == "" || state == "" {
		writeError(w, r, domain.NewAppError(domain.CodeUnauthorized, "missing oauth code/state", false))
		return
	}
	token, _, err := s.Auth.HandleCallback(r.Context(), provider, code, state)
	if err != nil {
		writeError(w, r, err)
		return
	}
	http.Redirect(w, r, s.Auth.FrontendCallbackURL(token), http.StatusFound)
}

func (s *Server) logout(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) me(w http.ResponseWriter, r *http.Request) {
	if user, ok := auth.UserFromContext(r.Context()); ok {
		writeJSON(w, http.StatusOK, map[string]any{"user": user})
		return
	}
	// Optional bearer on bypass/public paths.
	if h := r.Header.Get("Authorization"); strings.HasPrefix(h, "Bearer ") {
		user, err := s.Auth.Parse(strings.TrimPrefix(h, "Bearer "))
		if err == nil {
			writeJSON(w, http.StatusOK, map[string]any{"user": user})
			return
		}
	}
	if s.Auth.Bypass {
		writeJSON(w, http.StatusOK, map[string]any{
			"user": map[string]string{
				"email":    "dev@docforge.local",
				"name":     "DocForge Dev",
				"provider": "bypass",
			},
		})
		return
	}
	writeError(w, r, domain.NewAppError(domain.CodeUnauthorized, "authentication required", false))
}

func (s *Server) listDocuments(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	docs, err := s.Service.ListDocuments(r.Context(), limit)
	if err != nil {
		writeError(w, r, err)
		return
	}
	items := make([]map[string]any, 0, len(docs))
	for _, d := range docs {
		items = append(items, map[string]any{
			"document_id":    d.ID,
			"filename":       d.Filename,
			"content_type":   d.ContentType,
			"size_bytes":     d.SizeBytes,
			"page_count":     d.PageCount,
			"status":         d.Status,
			"output_formats": d.OutputFormats,
			"created_at":     d.CreatedAt,
			"updated_at":     d.UpdatedAt,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"documents": items})
}

func (s *Server) uploadQuota(w http.ResponseWriter, r *http.Request) {
	if s.Auth.Bypass {
		writeJSON(w, http.StatusOK, map[string]any{
			"tier":      "development",
			"limit":     s.AuthUploadLimit,
			"used":      0,
			"remaining": s.AuthUploadLimit,
		})
		return
	}
	subject := resolveQuotaSubject(w, r, s.AnonUploadLimit, s.AuthUploadLimit)
	used, remaining, err := s.Quota.Peek(r.Context(), subject.Key, subject.Limit)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"tier":      subject.Tier,
		"limit":     subject.Limit,
		"used":      used,
		"remaining": remaining,
	})
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

	var subject quotaSubject
	var remaining int
	if !s.Auth.Bypass {
		subject = resolveQuotaSubject(w, r, s.AnonUploadLimit, s.AuthUploadLimit)
		var consumeErr error
		remaining, consumeErr = s.Quota.Consume(r.Context(), subject.Key, subject.Limit)
		if consumeErr != nil {
			writeError(w, r, consumeErr)
			return
		}
	}

	result, err := s.Service.UploadDocument(r.Context(), application.UploadInput{
		Filename:      header.Filename,
		ContentType:   header.Header.Get("Content-Type"),
		SizeBytes:     header.Size,
		Body:          file,
		OutputFormats: formats,
	})
	if err != nil {
		if !s.Auth.Bypass {
			_ = s.Quota.Rollback(r.Context(), subject.Key)
		}
		writeError(w, r, err)
		return
	}
	if s.Metrics != nil {
		s.Metrics.UploadsTotal.Add(1)
		s.Metrics.JobsTotal.Add(1)
	}
	payload := map[string]any{
		"document_id": result.DocumentID,
		"job_id":      result.JobID,
		"status":      result.Status,
	}
	if !s.Auth.Bypass {
		payload["quota_tier"] = subject.Tier
		payload["quota_limit"] = subject.Limit
		payload["quota_remaining"] = remaining
	}
	writeJSON(w, http.StatusAccepted, payload)
}

func (s *Server) getJob(w http.ResponseWriter, r *http.Request) {
	job, err := s.Service.GetJob(r.Context(), chi.URLParam(r, "jobID"))
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJob(w, job)
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

func (s *Server) deleteDocument(w http.ResponseWriter, r *http.Request) {
	if err := s.Service.DeleteDocument(r.Context(), chi.URLParam(r, "documentID")); err != nil {
		writeError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) latestJob(w http.ResponseWriter, r *http.Request) {
	job, err := s.Service.LatestJobForDocument(r.Context(), chi.URLParam(r, "documentID"))
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJob(w, job)
}

func (s *Server) saveMarkdown(w http.ResponseWriter, r *http.Request) {
	raw, err := io.ReadAll(io.LimitReader(r.Body, s.Service.MaxBytes+1))
	if err != nil {
		writeError(w, r, domain.NewAppError(domain.CodeInternal, "failed to read body", true))
		return
	}
	if int64(len(raw)) > s.Service.MaxBytes && s.Service.MaxBytes > 0 {
		writeError(w, r, domain.NewAppError(domain.CodeFileTooLarge, "markdown exceeds size limit", false))
		return
	}
	if strings.Contains(strings.ToLower(r.Header.Get("Content-Type")), "json") {
		var payload struct {
			Markdown string `json:"markdown"`
		}
		if err := json.Unmarshal(raw, &payload); err != nil {
			writeError(w, r, domain.NewAppError(domain.CodeInternal, "invalid json body", false))
			return
		}
		raw = []byte(payload.Markdown)
	}
	art, err := s.Service.SaveMarkdown(r.Context(), chi.URLParam(r, "documentID"), raw)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"artifact_id": art.ID,
		"size_bytes":  art.SizeBytes,
		"format":      art.Format,
	})
}

func (s *Server) regenerateMarkdown(w http.ResponseWriter, r *http.Request) {
	art, err := s.Service.RegenerateMarkdown(r.Context(), chi.URLParam(r, "documentID"))
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"artifact_id": art.ID,
		"size_bytes":  art.SizeBytes,
		"format":      art.Format,
	})
}

func (s *Server) jobEvents(w http.ResponseWriter, r *http.Request) {
	flusher, _ := w.(http.Flusher)
	flush := func() {
		if flusher != nil {
			flusher.Flush()
		}
	}
	jobID := chi.URLParam(r, "jobID")
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	flush()

	send := func(event string, job *domain.Job) {
		payload := map[string]any{
			"job_id":      job.ID,
			"document_id": job.DocumentID,
			"status":      job.Status,
			"stage":       job.Stage,
			"progress":    job.Progress,
			"attempts":    job.Attempts,
			"created_at":  job.CreatedAt,
			"updated_at":  job.UpdatedAt,
		}
		if job.ErrorCode != "" {
			payload["error"] = map[string]any{"code": job.ErrorCode, "message": job.ErrorMsg}
		}
		raw, _ := json.Marshal(payload)
		fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, raw)
		flush()
	}

	job, err := s.Service.GetJob(r.Context(), jobID)
	if err != nil {
		raw, _ := json.Marshal(map[string]string{"message": err.Error()})
		fmt.Fprintf(w, "event: error\ndata: %s\n\n", raw)
		flush()
		return
	}
	send("progress", job)
	if job.Status == domain.JobCompleted || job.Status == domain.JobFailed || job.Status == domain.JobCancelled {
		send("done", job)
		return
	}

	tick := time.NewTicker(750 * time.Millisecond)
	defer tick.Stop()
	last := fmt.Sprintf("%s|%s|%d", job.Status, job.Stage, job.Progress)
	for {
		select {
		case <-r.Context().Done():
			return
		case <-tick.C:
			job, err = s.Service.GetJob(r.Context(), jobID)
			if err != nil {
				return
			}
			sig := fmt.Sprintf("%s|%s|%d", job.Status, job.Stage, job.Progress)
			if sig != last {
				last = sig
				send("progress", job)
			}
			if job.Status == domain.JobCompleted || job.Status == domain.JobFailed || job.Status == domain.JobCancelled {
				send("done", job)
				return
			}
		}
	}
}

func (s *Server) downloadOriginal(w http.ResponseWriter, r *http.Request) {
	doc, rc, err := s.Service.OpenOriginal(r.Context(), chi.URLParam(r, "documentID"))
	if err != nil {
		writeError(w, r, err)
		return
	}
	defer rc.Close()
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`inline; filename="%s"`, doc.Filename))
	if doc.SizeBytes > 0 {
		w.Header().Set("Content-Length", strconv.FormatInt(doc.SizeBytes, 10))
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
	if s.Metrics != nil {
		s.Metrics.JobsCancelled.Add(1)
	}
	writeJob(w, job)
}

func (s *Server) retryJob(w http.ResponseWriter, r *http.Request) {
	job, err := s.Service.RetryJob(r.Context(), chi.URLParam(r, "jobID"))
	if err != nil {
		writeError(w, r, err)
		return
	}
	if s.Metrics != nil {
		s.Metrics.JobsTotal.Add(1)
	}
	writeJSON(w, http.StatusAccepted, map[string]any{
		"job_id":      job.ID,
		"document_id": job.DocumentID,
		"status":      job.Status,
		"stage":       job.Stage,
		"progress":    job.Progress,
		"attempts":    job.Attempts,
	})
}

func writeJob(w http.ResponseWriter, job *domain.Job) {
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
		"attempts":    job.Attempts,
		"error":       jobErr,
		"created_at":  job.CreatedAt,
		"updated_at":  job.UpdatedAt,
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
			"artifact_id":  a.ID,
			"document_id":  a.DocumentID,
			"job_id":       a.JobID,
			"kind":         a.Kind,
			"format":       a.Format,
			"filename":     name,
			"size_bytes":   a.SizeBytes,
			"download_url": "/api/v1/artifacts/" + a.ID + "/download",
			"created_at":   a.CreatedAt,
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
	case domain.CodeUnauthorized:
		status = http.StatusUnauthorized
	case domain.CodeRateLimited, domain.CodeQuotaExceeded:
		status = http.StatusTooManyRequests
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
