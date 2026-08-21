package redisx

import (
	"context"
	"fmt"

	"github.com/redis/go-redis/v9"

	"github.com/thanhtai9606/DocForge/apps/api/internal/domain"
)

var consumeScript = redis.NewScript(`
local current = tonumber(redis.call('GET', KEYS[1]) or '0')
if current >= tonumber(ARGV[1]) then
  return -1
end
return redis.call('INCR', KEYS[1])
`)

// QuotaStore tracks upload counts in Redis.
type QuotaStore struct {
	client *redis.Client
}

func NewQuotaStore(client *redis.Client) *QuotaStore {
	return &QuotaStore{client: client}
}

func (s *QuotaStore) key(subject string) string {
	return fmt.Sprintf("docforge:quota:%s", subject)
}

func (s *QuotaStore) Consume(ctx context.Context, subject string, limit int) (int, error) {
	n, err := consumeScript.Run(ctx, s.client, []string{s.key(subject)}, limit).Int64()
	if err != nil {
		return 0, err
	}
	if n < 0 {
		return 0, domain.NewAppError(domain.CodeQuotaExceeded, "upload quota exceeded; sign in for a higher limit", false)
	}
	remaining := limit - int(n)
	if remaining < 0 {
		remaining = 0
	}
	return remaining, nil
}

func (s *QuotaStore) Rollback(ctx context.Context, subject string) error {
	key := s.key(subject)
	pipe := s.client.TxPipeline()
	pipe.Decr(ctx, key)
	pipe.Get(ctx, key)
	res, err := pipe.Exec(ctx)
	if err != nil {
		return err
	}
	if len(res) >= 2 {
		if v, err := res[1].(*redis.StringCmd).Int64(); err == nil && v < 0 {
			_ = s.client.Set(ctx, key, 0, 0).Err()
		}
	}
	return nil
}

func (s *QuotaStore) Peek(ctx context.Context, subject string, limit int) (int, int, error) {
	v, err := s.client.Get(ctx, s.key(subject)).Int64()
	if err == redis.Nil {
		return 0, limit, nil
	}
	if err != nil {
		return 0, 0, err
	}
	used := int(v)
	remaining := limit - used
	if remaining < 0 {
		remaining = 0
	}
	return used, remaining, nil
}
