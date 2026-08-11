package logging_test

import (
	"bytes"
	"context"
	"log/slog"
	"testing"

	"github.com/thanhtai9606/DocForge/apps/api/internal/logging"
)

func TestRequestIDRoundTrip(t *testing.T) {
	ctx := context.Background()
	if logging.RequestID(ctx) != "" {
		t.Fatal("expected empty request id")
	}
	ctx = logging.WithRequestID(ctx, "req-123")
	if logging.RequestID(ctx) != "req-123" {
		t.Fatalf("got %q", logging.RequestID(ctx))
	}
}

func TestFromContextAddsRequestID(t *testing.T) {
	var buf bytes.Buffer
	base := slog.New(slog.NewJSONHandler(&buf, nil))
	ctx := logging.WithRequestID(context.Background(), "abc")
	logging.FromContext(ctx, base).Info("hello")
	if !bytes.Contains(buf.Bytes(), []byte(`"request_id":"abc"`)) {
		t.Fatalf("log missing request_id: %s", buf.String())
	}
}

func TestNewLogger(t *testing.T) {
	if logging.New() == nil {
		t.Fatal("expected logger")
	}
}
