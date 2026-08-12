package metrics

import (
	"encoding/json"
	"net/http"
	"sync"
	"sync/atomic"
	"time"
)

// Registry holds process-level counters for Phase 5 observability.
type Registry struct {
	JobsTotal        atomic.Int64
	JobsSuccess      atomic.Int64
	JobsFailed       atomic.Int64
	JobsCancelled    atomic.Int64
	UploadsTotal     atomic.Int64
	ExportsTotal     atomic.Int64
	RateLimited      atomic.Int64
	HTTPRequests     atomic.Int64
	HTTPErrors       atomic.Int64
	PagesProcessed   atomic.Int64
	OCRTotal         atomic.Int64
	OCRErrors        atomic.Int64
	OCRDurationMS    atomic.Int64
	ExportErrors     atomic.Int64
	ExportDurationMS atomic.Int64
	ProviderErrors   atomic.Int64

	mu           sync.Mutex
	processingMS []int64
	startedAt    time.Time
}

func New() *Registry {
	return &Registry{startedAt: time.Now().UTC()}
}

func (r *Registry) ObserveProcessing(d time.Duration) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.processingMS = append(r.processingMS, d.Milliseconds())
	if len(r.processingMS) > 500 {
		r.processingMS = r.processingMS[len(r.processingMS)-500:]
	}
}

func (r *Registry) ObserveOCR(d time.Duration, err error) {
	r.OCRTotal.Add(1)
	r.OCRDurationMS.Add(d.Milliseconds())
	if err != nil {
		r.OCRErrors.Add(1)
		r.ProviderErrors.Add(1)
	}
}

func (r *Registry) ObserveExport(d time.Duration, err error) {
	r.ExportsTotal.Add(1)
	r.ExportDurationMS.Add(d.Milliseconds())
	if err != nil {
		r.ExportErrors.Add(1)
		r.ProviderErrors.Add(1)
	}
}

func (r *Registry) Snapshot() map[string]any {
	r.mu.Lock()
	defer r.mu.Unlock()
	var sum int64
	for _, v := range r.processingMS {
		sum += v
	}
	avg := float64(0)
	if n := len(r.processingMS); n > 0 {
		avg = float64(sum) / float64(n)
	}
	return map[string]any{
		"jobs_total":                 r.JobsTotal.Load(),
		"jobs_success":               r.JobsSuccess.Load(),
		"jobs_failed":                r.JobsFailed.Load(),
		"jobs_cancelled":             r.JobsCancelled.Load(),
		"uploads_total":              r.UploadsTotal.Load(),
		"exports_total":              r.ExportsTotal.Load(),
		"rate_limited_total":         r.RateLimited.Load(),
		"http_requests_total":        r.HTTPRequests.Load(),
		"http_errors_total":          r.HTTPErrors.Load(),
		"pages_processed_total":      r.PagesProcessed.Load(),
		"ocr_total":                  r.OCRTotal.Load(),
		"ocr_errors_total":           r.OCRErrors.Load(),
		"ocr_duration_ms_total":      r.OCRDurationMS.Load(),
		"export_errors_total":        r.ExportErrors.Load(),
		"export_duration_ms_total":   r.ExportDurationMS.Load(),
		"provider_errors_total":      r.ProviderErrors.Load(),
		"processing_duration_ms_avg": avg,
		"uptime_seconds":             time.Since(r.startedAt).Seconds(),
	}
}

func (r *Registry) Handler() http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(r.Snapshot())
	}
}
