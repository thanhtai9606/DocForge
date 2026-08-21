package memory

import (
	"context"
	"sync"

	"github.com/thanhtai9606/DocForge/apps/api/internal/domain"
)

// QuotaStore is an in-memory upload quota counter for tests.
type QuotaStore struct {
	mu    sync.Mutex
	count map[string]int
}

func NewQuotaStore() *QuotaStore {
	return &QuotaStore{count: map[string]int{}}
}

func (s *QuotaStore) Consume(_ context.Context, subject string, limit int) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	used := s.count[subject]
	if used >= limit {
		return 0, domain.NewAppError(domain.CodeQuotaExceeded, "upload quota exceeded; sign in for a higher limit", false)
	}
	s.count[subject] = used + 1
	return limit - s.count[subject], nil
}

func (s *QuotaStore) Rollback(_ context.Context, subject string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.count[subject] > 0 {
		s.count[subject]--
	}
	return nil
}

func (s *QuotaStore) Peek(_ context.Context, subject string, limit int) (int, int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	used := s.count[subject]
	remaining := limit - used
	if remaining < 0 {
		remaining = 0
	}
	return used, remaining, nil
}
