package middleware

import (
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/thanhtai9606/DocForge/apps/api/internal/metrics"
)

// SecurityHeaders adds baseline hardening headers.
func SecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Permissions-Policy", "geolocation=(), microphone=(), camera=()")
		w.Header().Set("Cross-Origin-Opener-Policy", "same-origin")
		next.ServeHTTP(w, r)
	})
}

// CORS allows the web app origin(s).
func CORS(allowedOrigins []string) func(http.Handler) http.Handler {
	allowAll := len(allowedOrigins) == 0 || (len(allowedOrigins) == 1 && allowedOrigins[0] == "*")
	set := map[string]struct{}{}
	for _, o := range allowedOrigins {
		set[strings.TrimSpace(o)] = struct{}{}
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")
			if origin != "" && (allowAll || containsOrigin(set, origin)) {
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Set("Vary", "Origin")
				w.Header().Set("Access-Control-Allow-Credentials", "true")
				w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, X-Request-ID")
				w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
				w.Header().Set("Access-Control-Expose-Headers", "X-Request-ID, Content-Disposition")
			}
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func containsOrigin(set map[string]struct{}, origin string) bool {
	_, ok := set[origin]
	return ok
}

// Timeout bounds request handling duration. SSE/event streams skip the deadline.
func Timeout(d time.Duration) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		timed := http.TimeoutHandler(next, d, `{"error":{"code":"TIMEOUT","message":"request timed out","retryable":true}}`)
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if strings.HasSuffix(r.URL.Path, "/events") {
				next.ServeHTTP(w, r)
				return
			}
			timed.ServeHTTP(w, r)
		})
	}
}

type visitor struct {
	tokens float64
	last   time.Time
}

// RateLimiter is a simple per-IP token bucket.
type RateLimiter struct {
	mu       sync.Mutex
	visitors map[string]*visitor
	rate     float64
	burst    float64
	metrics  *metrics.Registry
}

func NewRateLimiter(perMinute int, burst int, reg *metrics.Registry) *RateLimiter {
	if perMinute < 1 {
		perMinute = 120
	}
	if burst < 1 {
		burst = 30
	}
	return &RateLimiter{
		visitors: map[string]*visitor{},
		rate:     float64(perMinute) / 60.0,
		burst:    float64(burst),
		metrics:  reg,
	}
}

func (l *RateLimiter) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := clientIP(r)
		if !l.allow(ip) {
			if l.metrics != nil {
				l.metrics.RateLimited.Add(1)
			}
			w.Header().Set("Retry-After", "1")
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte(`{"error":{"code":"RATE_LIMITED","message":"too many requests","retryable":true}}`))
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (l *RateLimiter) allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	v, ok := l.visitors[key]
	if !ok {
		l.visitors[key] = &visitor{tokens: l.burst - 1, last: now}
		return true
	}
	elapsed := now.Sub(v.last).Seconds()
	v.tokens += elapsed * l.rate
	if v.tokens > l.burst {
		v.tokens = l.burst
	}
	v.last = now
	if v.tokens < 1 {
		return false
	}
	v.tokens--
	return true
}

func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		parts := strings.Split(xff, ",")
		return strings.TrimSpace(parts[0])
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// Metrics counts HTTP requests.
func Metrics(reg *metrics.Registry) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if reg != nil {
				reg.HTTPRequests.Add(1)
			}
			ww := &statusWriter{ResponseWriter: w, status: 200}
			next.ServeHTTP(ww, r)
			if reg != nil && ww.status >= 500 {
				reg.HTTPErrors.Add(1)
			}
		})
	}
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(code int) {
	w.status = code
	w.ResponseWriter.WriteHeader(code)
}

func (w *statusWriter) Flush() {
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}
