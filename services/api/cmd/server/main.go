package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/berzz26/recall/pkg/database"
	"github.com/berzz26/recall/services/api/internal/config"
	"github.com/berzz26/recall/services/api/internal/health"
	local_source "github.com/berzz26/recall/services/api/internal/local_source"
	"github.com/berzz26/recall/services/api/internal/processing"
	"github.com/berzz26/recall/services/api/internal/storage"
	"github.com/berzz26/recall/services/api/internal/video"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/recover"
)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))

	cfg := config.Load()

	db, err := database.New(cfg.DatabaseURL)
	if err != nil {
		slog.Error("failed to connect to database", "error", err)
		os.Exit(1)
	}
	defer db.Close()

	slog.Info("database connected", "env", cfg.Env)

	store, err := storage.NewLocalStorage(cfg.StorageRoot)
	if err != nil {
		slog.Error("failed to create storage", "error", err)
		os.Exit(1)
	}
	slog.Info("storage initialized", "root", store.Root())

	videoRepo := video.NewRepository(db.DB)
	videoService := video.NewServiceWithConfig(videoRepo, store, cfg.MaxUploadSize)
	videoHandler := video.NewHandler(videoService)

	localSourceRepo := local_source.NewRepository(db.DB)
	localSourceService := local_source.NewService(localSourceRepo, videoService, cfg.StabilityDuration)
	localSourceHandler := local_source.NewHandler(localSourceService)

	healthHandler := health.NewHandler(db.DB)

	if err := localSourceService.StartAllWatchers(context.Background()); err != nil {
		slog.Error("failed to start watchers", "error", err)
	}

	processor := &processing.NoopProcessor{}
	worker := processing.NewWorker(videoService, processor, cfg.PollInterval)
	workerCtx, workerCancel := context.WithCancel(context.Background())
	go worker.Start(workerCtx)

	app := fiber.New(fiber.Config{
		ErrorHandler: func(c *fiber.Ctx, err error) error {
			code := fiber.StatusInternalServerError
			if e, ok := err.(*fiber.Error); ok {
				code = e.Code
			}
			return c.Status(code).JSON(fiber.Map{"error": err.Error()})
		},
	})

	app.Use(recover.New())
	if cfg.Env == "development" {
		app.Use(logger.New(logger.Config{
			Format: "[${time}] ${status} - ${latency} ${method} ${path}\n",
		}))
	}

	app.Get("/health", healthHandler.Check)

	api := app.Group("/api")
	v1 := api.Group("/v1")
	v1.Mount("/videos", videoHandler.SetupRoutes())
	v1.Post("/ingest/local", videoHandler.IngestLocal)
	v1.Mount("/local-sources", localSourceHandler.SetupRoutes())

	sigCtx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	go func() {
		slog.Info("starting server", "addr", cfg.Addr, "env", cfg.Env)
		if err := app.Listen(cfg.Addr); err != nil {
			slog.Error("server listen error", "error", err)
			os.Exit(1)
		}
	}()

	<-sigCtx.Done()
	slog.Info("shutdown signal received, shutting down")

	workerCancel()
	localSourceService.StopAllWatchers()

	if err := app.Shutdown(); err != nil {
		slog.Error("graceful shutdown failed", "error", err)
		os.Exit(1)
	}

	db.Close()
	slog.Info("server stopped")
}
