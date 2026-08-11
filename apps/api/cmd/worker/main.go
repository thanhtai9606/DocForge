package main

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/thanhtai9606/DocForge/apps/api/internal/config"
	"github.com/thanhtai9606/DocForge/apps/api/internal/domain"
	"github.com/thanhtai9606/DocForge/apps/api/internal/infrastructure/postgres"
	"github.com/thanhtai9606/DocForge/apps/api/internal/infrastructure/rabbitmq"
	redisx "github.com/thanhtai9606/DocForge/apps/api/internal/infrastructure/redis"
	miniostore "github.com/thanhtai9606/DocForge/apps/api/internal/infrastructure/minio"
	"github.com/thanhtai9606/DocForge/apps/api/internal/jobs"
	"github.com/thanhtai9606/DocForge/apps/api/internal/logging"
	"github.com/thanhtai9606/DocForge/apps/api/internal/orchestrator"
	"github.com/thanhtai9606/DocForge/apps/api/internal/providers/stubocr"
	"github.com/thanhtai9606/DocForge/apps/api/internal/storage"
)

func main() {
	logger := logging.New()
	cfg, err := config.Load()
	if err != nil {
		logger.Error("config load failed", "error", err)
		os.Exit(1)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	docs, jobRepo, artifacts, objects, progress, consumer, cleanup, err := wireWorker(ctx, cfg, logger)
	if err != nil {
		logger.Error("worker wiring failed", "error", err)
		os.Exit(1)
	}
	defer cleanup()

	engine := &orchestrator.Engine{
		Documents: docs,
		Jobs:      jobRepo,
		Artifacts: artifacts,
		Objects:   objects,
		Progress:  progress,
		OCR:       stubocr.New(),
		Layout:    nil, // geometric default inside engine
		Exporters: nil, // json+markdown+docx defaults
	}

	logger.Info("worker listening", "queue", cfg.RabbitQueue)
	err = consumer.Serve(ctx, func(cctx context.Context, msg rabbitmq.ProcessJobMessage) error {
		log := logger.With("job_id", msg.JobID, "document_id", msg.DocumentID)
		log.Info("processing job")
		res, runErr := engine.Run(cctx, msg.JobID)
		if runErr != nil {
			if appErr, ok := domain.AsAppError(runErr); ok && !appErr.Retryable {
				log.Error("job failed permanently", "error", runErr)
				return nil
			}
			log.Error("job failed retryable", "error", runErr)
			return runErr
		}
		log.Info("job completed", "kind", res.Kind, "cdom_key", res.CDOMKey, "artifacts", len(res.ArtifactIDs))
		return nil
	})
	if err != nil && !errors.Is(err, context.Canceled) {
		logger.Error("consumer stopped", "error", err)
		os.Exit(1)
	}
}

func wireWorker(ctx context.Context, cfg config.Config, logger *slog.Logger) (
	jobs.DocumentRepository,
	jobs.JobRepository,
	jobs.ArtifactRepository,
	storage.ObjectStore,
	jobs.ProgressStore,
	*rabbitmq.Consumer,
	func(),
	error,
) {
	db, err := postgres.Open(cfg.DatabaseURL)
	if err != nil {
		return nil, nil, nil, nil, nil, nil, nil, err
	}
	if err := postgres.EnsureSchema(ctx, db); err != nil {
		_ = db.Close()
		return nil, nil, nil, nil, nil, nil, nil, err
	}
	store := postgres.New(db)

	redisClient, err := redisx.NewClient(cfg.RedisURL)
	if err != nil {
		_ = db.Close()
		return nil, nil, nil, nil, nil, nil, nil, err
	}
	progress := redisx.NewProgressStore(redisClient)

	consumer, err := rabbitmq.ConnectConsumer(cfg.RabbitURL, cfg.RabbitExchange, cfg.RabbitQueue, cfg.RabbitRoutingKey)
	if err != nil {
		_ = redisClient.Close()
		_ = db.Close()
		return nil, nil, nil, nil, nil, nil, nil, err
	}

	objectStore := miniostore.New(cfg)
	cleanup := func() {
		_ = consumer.Close()
		_ = redisClient.Close()
		_ = db.Close()
	}
	logger.Info("worker dependencies ready")

	return postgres.NewDocumentRepo(store), postgres.NewJobRepo(store), postgres.NewArtifactRepo(store), objectStore, progress, consumer, cleanup, nil
}
