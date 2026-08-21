package api

import (
	"net/http"
	"strings"

	"github.com/thanhtai9606/DocForge/apps/api/internal/auth"
)

type quotaSubject struct {
	Key   string
	Limit int
	Tier  string
}

func resolveQuotaSubject(w http.ResponseWriter, r *http.Request, anonLimit, authLimit int) quotaSubject {
	if user, ok := auth.UserFromContext(r.Context()); ok && strings.TrimSpace(user.Email) != "" {
		email := strings.ToLower(strings.TrimSpace(user.Email))
		return quotaSubject{
			Key:   "user:" + email,
			Limit: authLimit,
			Tier:  "authenticated",
		}
	}
	return quotaSubject{
		Key:   "anon:" + auth.GuestID(w, r),
		Limit: anonLimit,
		Tier:  "anonymous",
	}
}
