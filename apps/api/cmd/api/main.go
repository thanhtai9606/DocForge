package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/thanhtai9606/DocForge/apps/api/internal/api"
	"github.com/thanhtai9606/DocForge/apps/api/internal/application"
	"github.com/thanhtai9606/DocForge/apps/api/internal/auth"
	"github.com/thanhtai9606/DocForge/apps/api/internal/cleanup"
	"github.com/thanhtai9606/DocForge/apps/api/internal/config"
	"github.com/thanhtai9606/DocForge/apps/api/internal/infrastructure/memory"
	"github.com/thanhtai9606/DocForge/apps/api/internal/infrastructure/postgres"
	"github.com/thanhtai9606/DocForge/apps/api/internal/infrastructure/rabbitmq"
	redisx "github.com/thanhtai9606/DocForge/apps/api/internal/infrastructure/redis"
	miniostore "github.com/thanhtai9606/DocForge/apps/api/internal/infrastructure/minio"
	"github.com/thanhtai9606/DocForge/apps/api/internal/jobs"
	"github.com/thanhtai9606/DocForge/apps/api/internal/logging"
	"github.com/thanhtai9606/DocForge/apps/api/internal/metrics"
	"github.com/thanhtai9606/DocForge/apps/api/internal/quota"
	"github.com/thanhtai9606/DocForge/apps/api/internal/storage"
)

func main() {
	logger := logging.New()
	cfg, err := config.Load()
	if err != nil {
		logger.Error("config load failed", "error", err)
		os.Exit(1)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	docs, jobRepo, artifacts, objects, queue, progress, quotaStore, cleanupFn, err := wireDeps(ctx, cfg, logger)
	if err != nil {
		logger.Error("dependency wiring failed", "error", err)
		os.Exit(1)
	}
	defer cleanupFn()

	reg := metrics.New()
	svc := &application.Service{
		Documents:   docs,
		Jobs:        jobRepo,
		Artifacts:   artifacts,
		Objects:     objects,
		Queue:       queue,
		Progress:    progress,
		MaxBytes:    cfg.MaxUploadBytes,
		MaxPages:    cfg.MaxPages,
		MaxAttempts: cfg.MaxAttempts,
	}

	sweeper := &cleanup.Sweeper{
		Documents: docs,
		Jobs:      jobRepo,
		Artifacts: artifacts,
		Objects:   objects,
		TTL:       cfg.ArtifactTTL,
		Logger:    logger,
	}
	sweeper.Start(ctx, cfg.CleanupInterval)

	handler := api.NewServer(svc, api.Options{
		Auth: auth.New(
			cfg.AuthSecret,
			cfg.DevAuthDisabled,
			cfg.WebOrigin,
			cfg.APIPublicOrigin,
			auth.ProviderConfig{ClientID: cfg.GoogleClientID, ClientSecret: cfg.GoogleClientSecret},
			auth.ProviderConfig{ClientID: cfg.MicrosoftClientID, ClientSecret: cfg.MicrosoftClientSecret, Tenant: cfg.MicrosoftTenant},
		),
		Metrics:         reg,
		Quota:           quotaStore,
		AnonUploadLimit: cfg.AnonUploadLimit,
		AuthUploadLimit: cfg.AuthUploadLimit,
		CORSOrigins:     cfg.CORSOrigins,
		RequestTimeout:  cfg.RequestTimeout,
		RatePerMinute:   cfg.RatePerMinute,
		RateBurst:       cfg.RateBurst,
	})

	server := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		logger.Info("api listening", "addr", cfg.HTTPAddr)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("server failed", "error", err)
			os.Exit(1)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop
	cancel()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer shutdownCancel()
	_ = server.Shutdown(shutdownCtx)
}

func wireDeps(ctx context.Context, cfg config.Config, logger *slog.Logger) (
	jobs.DocumentRepository,
	jobs.JobRepository,
	jobs.ArtifactRepository,
	storage.ObjectStore,
	jobs.QueuePublisher,
	jobs.ProgressStore,
	quota.Store,
	func(),
	error,
) {
	useMemory := os.Getenv("DOCFORGE_USE_MEMORY") == "1"

	cleanupFn := func() {}
	if useMemory {
		logger.Warn("running with in-memory adapters (DOCFORGE_USE_MEMORY=1)")
		return memory.NewDocumentRepo(), memory.NewJobRepo(), memory.NewArtifactRepo(), memory.NewObjectStore(), memory.NewQueue(), memory.NewProgress(), memory.NewQuotaStore(), cleanupFn, nil
	}

	db, err := postgres.Open(cfg.DatabaseURL)
	if err != nil {
		return nil, nil, nil, nil, nil, nil, nil, nil, err
	}
	if err := postgres.EnsureSchema(ctx, db); err != nil {
		_ = db.Close()
		return nil, nil, nil, nil, nil, nil, nil, nil, err
	}
	store := postgres.New(db)

	redisClient, err := redisx.NewClient(cfg.RedisURL)
	if err != nil {
		_ = db.Close()
		return nil, nil, nil, nil, nil, nil, nil, nil, err
	}
	progress := redisx.NewProgressStore(redisClient)
	quotaStore := redisx.NewQuotaStore(redisClient)

	publisher, err := rabbitmq.Connect(cfg.RabbitURL, cfg.RabbitExchange, cfg.RabbitQueue, cfg.RabbitRoutingKey)
	if err != nil {
		_ = redisClient.Close()
		_ = db.Close()
		return nil, nil, nil, nil, nil, nil, nil, nil, err
	}

	objectStore := miniostore.New(cfg)
	if err := objectStore.EnsureBucket(ctx); err != nil {
		logger.Warn("ensure minio bucket failed; continuing", "error", err)
	}

	cleanupFn = func() {
		_ = publisher.Close()
		_ = redisClient.Close()
		_ = db.Close()
	}

	return postgres.NewDocumentRepo(store), postgres.NewJobRepo(store), postgres.NewArtifactRepo(store), objectStore, publisher, progress, quotaStore, cleanupFn, nil
}
