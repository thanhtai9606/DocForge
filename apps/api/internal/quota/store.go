package quota

import "context"

// Store tracks upload usage per subject key (e.g. anon:<guest-id>, user:<email>).
type Store interface {
	Consume(ctx context.Context, subject string, limit int) (remaining int, err error)
	Rollback(ctx context.Context, subject string) error
	Peek(ctx context.Context, subject string, limit int) (used, remaining int, err error)
}
