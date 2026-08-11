package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config holds runtime configuration for the API process.
type Config struct {
	HTTPAddr           string
	DatabaseURL        string
	RedisURL           string
	RabbitURL          string
	MinIOEndpoint      string
	MinIORegion        string
	MinIOBucket        string
	MinIOAccessKey     string
	MinIOSecretKey     string
	MinIOUsePathStyle  bool
	MaxUploadBytes     int64
	MaxPages           int
	ShutdownTimeout    time.Duration
	RabbitExchange     string
	RabbitQueue        string
	RabbitRoutingKey   string
	DevAuthDisabled    bool
}

// Load reads configuration from environment variables.
func Load() (Config, error) {
	cfg := Config{
		HTTPAddr:          getenv("HTTP_ADDR", ":8080"),
		DatabaseURL:       getenv("DATABASE_URL", "postgres://docforge:docforge@localhost:5432/docforge?sslmode=disable"),
		RedisURL:          getenv("REDIS_URL", "redis://localhost:6379/0"),
		RabbitURL:         getenv("RABBITMQ_URL", "amqp://docforge:docforge@localhost:5672/"),
		MinIOEndpoint:     firstNonEmpty(os.Getenv("MINIO_ENDPOINT"), "http://localhost:9000"),
		MinIORegion:       firstNonEmpty(os.Getenv("MINIO_REGION"), "us-east-1"),
		MinIOBucket:       firstNonEmpty(os.Getenv("MINIO_BUCKET"), "docforge"),
		MinIOAccessKey:    firstNonEmpty(os.Getenv("MINIO_ACCESS_KEY"), "docforge"),
		MinIOSecretKey:    firstNonEmpty(os.Getenv("MINIO_SECRET_KEY"), "docforge_secret"),
		MinIOUsePathStyle: getenvBool("MINIO_USE_PATH_STYLE", true),
		MaxUploadBytes:    getenvInt64("MAX_UPLOAD_BYTES", 50<<20),
		MaxPages:          getenvInt("MAX_PAGES", 100),
		ShutdownTimeout:   getenvDuration("SHUTDOWN_TIMEOUT", 10*time.Second),
		RabbitExchange:    getenv("RABBITMQ_EXCHANGE", "docforge.jobs"),
		RabbitQueue:       getenv("RABBITMQ_QUEUE", "docforge.jobs.process"),
		RabbitRoutingKey:  getenv("RABBITMQ_ROUTING_KEY", "process"),
		DevAuthDisabled:   getenvBool("DEV_AUTH_DISABLED", true),
	}
	if cfg.MinIOBucket == "" {
		return Config{}, fmt.Errorf("MINIO_BUCKET is required")
	}
	return cfg, nil
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func getenv(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}

func getenvBool(key string, fallback bool) bool {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return fallback
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return fallback
	}
	return b
}

func getenvInt(key string, fallback int) int {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return fallback
	}
	return n
}

func getenvInt64(key string, fallback int64) int64 {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return fallback
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		return fallback
	}
	return n
}

func getenvDuration(key string, fallback time.Duration) time.Duration {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return fallback
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return fallback
	}
	return d
}
