package redisx

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
)

// ProgressStore mirrors job progress in Redis.
type ProgressStore struct {
	client *redis.Client
	ttl    time.Duration
}

func NewClient(redisURL string) (*redis.Client, error) {
	opt, err := redis.ParseURL(redisURL)
	if err != nil {
		return nil, err
	}
	client := redis.NewClient(opt)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := client.Ping(ctx).Err(); err != nil {
		_ = client.Close()
		return nil, err
	}
	return client, nil
}

func NewProgressStore(client *redis.Client) *ProgressStore {
	return &ProgressStore{client: client, ttl: 24 * time.Hour}
}

func (s *ProgressStore) key(jobID string) string {
	return fmt.Sprintf("job:%s:progress", jobID)
}

func (s *ProgressStore) SetProgress(ctx context.Context, jobID, stage string, progress int) error {
	pipe := s.client.TxPipeline()
	key := s.key(jobID)
	pipe.HSet(ctx, key, map[string]any{
		"stage":    stage,
		"progress": progress,
	})
	pipe.Expire(ctx, key, s.ttl)
	_, err := pipe.Exec(ctx)
	return err
}

func (s *ProgressStore) GetProgress(ctx context.Context, jobID string) (string, int, bool, error) {
	vals, err := s.client.HGetAll(ctx, s.key(jobID)).Result()
	if err != nil {
		return "", 0, false, err
	}
	if len(vals) == 0 {
		return "", 0, false, nil
	}
	progress, _ := strconv.Atoi(vals["progress"])
	return vals["stage"], progress, true, nil
}
