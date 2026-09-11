package main

import (
	"context"
	"log/slog"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/berzz26/recall/pkg/database"
	"github.com/berzz26/recall/services/api/internal/config"
	"github.com/berzz26/recall/services/api/internal/detection"
	"github.com/berzz26/recall/services/api/internal/detector"
	"github.com/berzz26/recall/services/api/internal/handlers"
	"github.com/berzz26/recall/services/api/internal/health"
	local_source "github.com/berzz26/recall/services/api/internal/local_source"
	"github.com/berzz26/recall/services/api/internal/processing"
	"github.com/berzz26/recall/services/api/internal/storage"
	"github.com/berzz26/recall/services/api/internal/video"
	"github.com/berzz26/recall/services/api/internal/video_frame"
	"github.com/berzz26/recall/services/api/internal/video_media"
	"github.com/berzz26/recall/services/api/internal/video_segment"
	"github.com/berzz26/recall/services/api/internal/visual"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
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

	videoMediaRepo := video_media.NewRepository(db.DB)
	videoMediaService := video_media.NewService(videoMediaRepo)

	videoSegmentRepo := video_segment.NewRepository(db.DB)
	videoSegmentService := video_segment.NewService(videoSegmentRepo, cfg.SegmentDuration)

	videoFrameRepo := video_frame.NewRepository(db.DB)
	videoFrameService := video_frame.NewService(videoFrameRepo, store, cfg.FrameSampleInterval, cfg.FFmpegPath, cfg.FFmpegTimeout, cfg.FrameJPEGQuality)

	detectionRepo := detection.NewRepository(db.DB)
	scriptPath := filepath.Join("workers", "detector", "detect.py")
	if _, err := os.Stat(scriptPath); err != nil {
		if abs, err2 := filepath.Abs(scriptPath); err2 == nil {
			if _, err3 := os.Stat(abs); err3 == nil {
				scriptPath = abs
			}
		}
		if _, err := os.Stat(scriptPath); err != nil {
			alt := "/home/berzz/recall/workers/detector/detect.py"
			if _, err2 := os.Stat(alt); err2 == nil {
				scriptPath = alt
			}
		}
	} else {
		if abs, err := filepath.Abs(scriptPath); err == nil {
			scriptPath = abs
		}
	}
	yolo := detector.NewYoloDetector(cfg.PythonPath, scriptPath, cfg.ModelPath, cfg.DetectionThreshold)
	visualService := visual.NewService(detectionRepo, videoFrameRepo, store, yolo, cfg.DetectionThreshold, cfg.DetectorName, cfg.DetectorVersion)

	localSourceRepo := local_source.NewRepository(db.DB)
	localSourceService := local_source.NewService(localSourceRepo, videoService, cfg.StabilityDuration)
	localSourceHandler := local_source.NewHandler(localSourceService)

	healthHandler := health.NewHandler(db.DB)

	if err := localSourceService.StartAllWatchers(context.Background()); err != nil {
		slog.Error("failed to start watchers", "error", err)
	}

	if _, err := os.Stat(cfg.FFprobePath); err != nil {
		if _, err2 := exec.LookPath(cfg.FFprobePath); err2 != nil {
			slog.Warn("ffprobe not found, processing will fail", "path", cfg.FFprobePath, "error", err2)
		}
	}

	if _, err := exec.LookPath(cfg.FFmpegPath); err != nil {
		slog.Warn("ffmpeg not found, frame extraction will fail", "path", cfg.FFmpegPath, "error", err)
	}

	processor := processing.NewFFprobeProcessorWithVisual(cfg.FFprobePath, cfg.FFprobeTimeout, store, videoMediaService, videoSegmentService, videoFrameService, visualService)
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
	app.Use(cors.New(cors.Config{
		AllowOrigins: "*",
		AllowHeaders: "Origin, Content-Type, Accept, Authorization",
		AllowMethods: "GET,POST,DELETE,OPTIONS",
	}))
	if cfg.Env == "development" {
		app.Use(logger.New(logger.Config{
			Format: "[${time}] ${status} - ${latency} ${method} ${path}\n",
		}))
	}

	app.Get("/health", healthHandler.Check)

	detailHandler := handlers.NewVideoDetailHandler(videoMediaRepo, videoSegmentRepo, videoFrameRepo, detectionRepo, store)

	api := app.Group("/api")
	v1 := api.Group("/v1")
	v1.Mount("/videos", videoHandler.SetupRoutes())
	v1.Get("/videos/:id/media", detailHandler.GetMedia)
	v1.Get("/videos/:id/segments", detailHandler.GetSegments)
	v1.Get("/videos/:id/frames", detailHandler.GetFrames)
	v1.Get("/videos/:id/detections", detailHandler.GetDetections)
	v1.Get("/videos/:id/frames/:frameId/image", detailHandler.GetFrameImage)
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
