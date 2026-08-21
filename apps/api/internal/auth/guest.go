package auth

import (
	"net/http"
	"strings"

	"github.com/google/uuid"
)

const GuestCookieName = "docforge_guest"

// GuestID returns a stable anonymous client id, setting a cookie when missing.
func GuestID(w http.ResponseWriter, r *http.Request) string {
	if c, err := r.Cookie(GuestCookieName); err == nil {
		if id := strings.TrimSpace(c.Value); id != "" {
			return id
		}
	}
	id := uuid.NewString()
	http.SetCookie(w, &http.Cookie{
		Name:     GuestCookieName,
		Value:    id,
		Path:     "/",
		MaxAge:   365 * 24 * 3600,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
	return id
}

// AllowsGuestAccess marks routes usable without SSO (upload + follow processing).
func AllowsGuestAccess(r *http.Request) bool {
	path := r.URL.Path
	if r.Method == http.MethodPost && path == "/api/v1/documents" {
		return true
	}
	if path == "/api/v1/quota" {
		return true
	}
	if r.Method != http.MethodGet {
		return false
	}
	switch {
	case strings.HasPrefix(path, "/api/v1/jobs/"):
		return true
	case strings.HasPrefix(path, "/api/v1/documents/"):
		return true
	case strings.HasPrefix(path, "/api/v1/artifacts/"):
		return true
	default:
		return false
	}
}
