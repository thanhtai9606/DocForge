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
	HTTPAddr              string
	DatabaseURL           string
	RedisURL              string
	RabbitURL             string
	MinIOEndpoint         string
	MinIORegion           string
	MinIOBucket           string
	MinIOAccessKey        string
	MinIOSecretKey        string
	MinIOUsePathStyle     bool
	MaxUploadBytes        int64
	MaxPages              int
	MaxAttempts           int
	ShutdownTimeout       time.Duration
	RequestTimeout        time.Duration
	RabbitExchange        string
	RabbitQueue           string
	RabbitRoutingKey      string
	DevAuthDisabled       bool
	AuthSecret            string
	WebOrigin             string
	APIPublicOrigin       string
	GoogleClientID        string
	GoogleClientSecret    string
	MicrosoftClientID     string
	MicrosoftClientSecret string
	MicrosoftTenant       string
	CORSOrigins           []string
	RatePerMinute         int
	RateBurst             int
	AnonUploadLimit       int
	AuthUploadLimit       int
	ArtifactTTL           time.Duration
	CleanupInterval       time.Duration
	OCRProvider           string
	OCRURL                string
	OCRTimeout            time.Duration
}

// Load reads configuration from environment variables.
func Load() (Config, error) {
	cfg := Config{
		HTTPAddr:              getenv("HTTP_ADDR", ":8080"),
		DatabaseURL:           getenv("DATABASE_URL", "postgres://docforge:docforge@localhost:5432/docforge?sslmode=disable"),
		RedisURL:              getenv("REDIS_URL", "redis://localhost:6379/0"),
		RabbitURL:             getenv("RABBITMQ_URL", "amqp://docforge:docforge@localhost:5672/"),
		MinIOEndpoint:         firstNonEmpty(os.Getenv("MINIO_ENDPOINT"), "http://localhost:9000"),
		MinIORegion:           firstNonEmpty(os.Getenv("MINIO_REGION"), "us-east-1"),
		MinIOBucket:           firstNonEmpty(os.Getenv("MINIO_BUCKET"), "docforge"),
		MinIOAccessKey:        firstNonEmpty(os.Getenv("MINIO_ACCESS_KEY"), "docforge"),
		MinIOSecretKey:        firstNonEmpty(os.Getenv("MINIO_SECRET_KEY"), "docforge_secret"),
		MinIOUsePathStyle:     getenvBool("MINIO_USE_PATH_STYLE", true),
		MaxUploadBytes:        getenvInt64("MAX_UPLOAD_BYTES", 50<<20),
		MaxPages:              getenvInt("MAX_PAGES", 100),
		MaxAttempts:           getenvInt("MAX_ATTEMPTS", 3),
		ShutdownTimeout:       getenvDuration("SHUTDOWN_TIMEOUT", 10*time.Second),
		RequestTimeout:        getenvDuration("REQUEST_TIMEOUT", 60*time.Second),
		RabbitExchange:        getenv("RABBITMQ_EXCHANGE", "docforge.jobs"),
		RabbitQueue:           getenv("RABBITMQ_QUEUE", "docforge.jobs.process"),
		RabbitRoutingKey:      getenv("RABBITMQ_ROUTING_KEY", "process"),
		DevAuthDisabled:       getenvBool("AUTH_BYPASS", getenvBool("DEV_AUTH_DISABLED", true)),
		AuthSecret:            getenv("AUTH_SECRET", "docforge-dev-secret-change-me"),
		WebOrigin:             getenv("WEB_ORIGIN", "http://localhost:5173"),
		APIPublicOrigin:       getenv("API_PUBLIC_ORIGIN", "http://localhost:8080"),
		GoogleClientID:        firstNonEmpty(os.Getenv("GOOGLE_OAUTH_CLIENT_ID"), os.Getenv("GOOGLE_CLIENT_ID")),
		GoogleClientSecret:    firstNonEmpty(os.Getenv("GOOGLE_OAUTH_CLIENT_SECRET"), os.Getenv("GOOGLE_CLIENT_SECRET")),
		MicrosoftClientID:     getenv("MICROSOFT_OAUTH_CLIENT_ID", ""),
		MicrosoftClientSecret: getenv("MICROSOFT_OAUTH_CLIENT_SECRET", ""),
		MicrosoftTenant:       getenv("MICROSOFT_OAUTH_TENANT", "common"),
		CORSOrigins:           splitCSV(getenv("CORS_ORIGINS", "http://localhost:5173,http://127.0.0.1:5173")),
		RatePerMinute:         getenvInt("RATE_LIMIT_PER_MINUTE", 180),
		RateBurst:             getenvInt("RATE_LIMIT_BURST", 40),
		AnonUploadLimit:       getenvInt("ANON_UPLOAD_LIMIT", 3),
		AuthUploadLimit:       getenvInt("AUTH_UPLOAD_LIMIT", 10),
		ArtifactTTL:           getenvDuration("ARTIFACT_TTL", 168*time.Hour),
		CleanupInterval:       getenvDuration("CLEANUP_INTERVAL", time.Hour),
		OCRProvider:           getenv("OCR_PROVIDER", "auto"),
		OCRURL:                getenv("OCR_URL", ""),
		OCRTimeout:            getenvDuration("OCR_TIMEOUT", 90*time.Second),
	}
	if cfg.MinIOBucket == "" {
		return Config{}, fmt.Errorf("MINIO_BUCKET is required")
	}
	return cfg, nil
}

func splitCSV(v string) []string {
	parts := strings.Split(v, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
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
