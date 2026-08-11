package config_test

import (
	"testing"
	"time"

	"github.com/thanhtai9606/DocForge/apps/api/internal/config"
)

func TestLoadDefaults(t *testing.T) {
	t.Setenv("HTTP_ADDR", "")
	t.Setenv("DATABASE_URL", "")
	t.Setenv("REDIS_URL", "")
	t.Setenv("RABBITMQ_URL", "")
	t.Setenv("MINIO_ENDPOINT", "")
	t.Setenv("MINIO_REGION", "")
	t.Setenv("MINIO_BUCKET", "")
	t.Setenv("MINIO_ACCESS_KEY", "")
	t.Setenv("MINIO_SECRET_KEY", "")
	t.Setenv("MINIO_USE_PATH_STYLE", "")
	t.Setenv("MAX_UPLOAD_BYTES", "")
	t.Setenv("MAX_PAGES", "")
	t.Setenv("SHUTDOWN_TIMEOUT", "")
	t.Setenv("DEV_AUTH_DISABLED", "")

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.HTTPAddr != ":8080" {
		t.Fatalf("HTTPAddr=%q", cfg.HTTPAddr)
	}
	if cfg.MinIOBucket != "docforge" {
		t.Fatalf("MinIOBucket=%q", cfg.MinIOBucket)
	}
	if !cfg.MinIOUsePathStyle {
		t.Fatal("expected path style true")
	}
	if cfg.MaxUploadBytes != 50<<20 {
		t.Fatalf("MaxUploadBytes=%d", cfg.MaxUploadBytes)
	}
	if cfg.MaxPages != 100 {
		t.Fatalf("MaxPages=%d", cfg.MaxPages)
	}
	if cfg.ShutdownTimeout != 10*time.Second {
		t.Fatalf("ShutdownTimeout=%s", cfg.ShutdownTimeout)
	}
	if !cfg.DevAuthDisabled {
		t.Fatal("expected DevAuthDisabled default true")
	}
}

func TestLoadFromEnv(t *testing.T) {
	t.Setenv("HTTP_ADDR", ":9090")
	t.Setenv("MINIO_BUCKET", "docs")
	t.Setenv("MINIO_USE_PATH_STYLE", "false")
	t.Setenv("MAX_UPLOAD_BYTES", "1024")
	t.Setenv("MAX_PAGES", "7")
	t.Setenv("SHUTDOWN_TIMEOUT", "3s")
	t.Setenv("DEV_AUTH_DISABLED", "false")
	t.Setenv("RABBITMQ_EXCHANGE", "ex")
	t.Setenv("RABBITMQ_QUEUE", "q")
	t.Setenv("RABBITMQ_ROUTING_KEY", "rk")

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.HTTPAddr != ":9090" || cfg.MinIOBucket != "docs" {
		t.Fatalf("unexpected cfg: %+v", cfg)
	}
	if cfg.MinIOUsePathStyle {
		t.Fatal("expected path style false")
	}
	if cfg.MaxUploadBytes != 1024 || cfg.MaxPages != 7 {
		t.Fatalf("limits wrong: %+v", cfg)
	}
	if cfg.ShutdownTimeout != 3*time.Second {
		t.Fatalf("timeout=%s", cfg.ShutdownTimeout)
	}
	if cfg.DevAuthDisabled {
		t.Fatal("expected DevAuthDisabled false")
	}
	if cfg.RabbitExchange != "ex" || cfg.RabbitQueue != "q" || cfg.RabbitRoutingKey != "rk" {
		t.Fatalf("rabbit cfg wrong: %+v", cfg)
	}
}
