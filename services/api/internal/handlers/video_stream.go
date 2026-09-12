package handlers

import (
	"context"
	"io"
	"os"
	"time"

	"github.com/berzz26/recall/services/api/internal/storage"
	"github.com/berzz26/recall/services/api/internal/video"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

type VideoStreamHandler struct {
	videoRepo *video.Repository
	storage   storage.Storage
}

func NewVideoStreamHandler(videoRepo *video.Repository, storage storage.Storage) *VideoStreamHandler {
	return &VideoStreamHandler{videoRepo: videoRepo, storage: storage}
}

func (h *VideoStreamHandler) Stream(c *fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid id"})
	}
	ctx, cancel := context.WithTimeout(c.UserContext(), 10*time.Second)
	defer cancel()
	v, err := h.videoRepo.GetByID(ctx, id)
	if err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "video not found"})
	}
	// Prefer storage key for UPLOAD, else source_path for LOCAL
	if v.StorageKey != nil && *v.StorageKey != "" {
		rc, err := h.storage.Open(ctx, *v.StorageKey)
		if err == nil {
			defer rc.Close()
			c.Set("Content-Type", v.MimeType)
			if v.MimeType == "" {
				c.Set("Content-Type", "video/mp4")
			}
			c.Set("Accept-Ranges", "bytes")
			data, err := io.ReadAll(rc)
			if err != nil {
				return c.Status(500).JSON(fiber.Map{"error": "failed to read video"})
			}
			return c.Send(data)
		}
	}
	if v.SourcePath != nil && *v.SourcePath != "" {
		f, err := os.Open(*v.SourcePath)
		if err == nil {
			defer f.Close()
			c.Set("Content-Type", v.MimeType)
			if v.MimeType == "" {
				c.Set("Content-Type", "video/mp4")
			}
			c.Set("Accept-Ranges", "bytes")
			data, err := io.ReadAll(f)
			if err != nil {
				return c.Status(500).JSON(fiber.Map{"error": "failed to read video"})
			}
			return c.Send(data)
		}
	}
	return c.Status(404).JSON(fiber.Map{"error": "video file not found"})
}
