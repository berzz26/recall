package video

import (
	"context"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

type Handler struct {
	service *Service
}

func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

func (h *Handler) Create(c *fiber.Ctx) error {
	contentType := c.Get("Content-Type")
	if strings.Contains(contentType, "multipart/form-data") {
		file, err := c.FormFile("file")
		if err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "file is required"})
		}
		src, err := file.Open()
		if err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "failed to open file"})
		}
		defer src.Close()

		mimeType := file.Header.Get("Content-Type")
		if mimeType == "" {
			mimeType = "application/octet-stream"
		}

		ctx, cancel := context.WithTimeout(c.UserContext(), 30*time.Second)
		defer cancel()

		v, err := h.service.UploadVideo(ctx, file.Filename, src, mimeType)
		if err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
		}
		return c.Status(fiber.StatusCreated).JSON(v)
	}

	var req CreateVideoRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
	}
	if req.Filename == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "filename is required"})
	}
	if req.SourceType != nil && *req.SourceType != SourceTypeLocal && *req.SourceType != SourceTypeUpload {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid source_type"})
	}
	if req.SourceType != nil && *req.SourceType == SourceTypeLocal && (req.SourcePath == nil || *req.SourcePath == "") {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "source_path is required for LOCAL source_type"})
	}

	ctx, cancel := context.WithTimeout(c.UserContext(), 5*time.Second)
	defer cancel()

	v, err := h.service.CreateVideo(ctx, req.Filename, req.ContentHash, req.MimeType, req.SizeBytes, req.SourceType, req.SourcePath)
	if err != nil {
		if strings.Contains(err.Error(), "does not exist") {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": err.Error()})
		}
		if strings.Contains(err.Error(), "not a regular file") || strings.Contains(err.Error(), "unsupported file type") || strings.Contains(err.Error(), "source_path is required") {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
		}
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.Status(fiber.StatusCreated).JSON(v)
}

func (h *Handler) List(c *fiber.Ctx) error {
	ctx, cancel := context.WithTimeout(c.UserContext(), 5*time.Second)
	defer cancel()

	videos, err := h.service.ListVideos(ctx, 20, 0)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to list videos"})
	}
	return c.JSON(videos)
}

func (h *Handler) Get(c *fiber.Ctx) error {
	idStr := c.Params("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid id"})
	}

	ctx, cancel := context.WithTimeout(c.UserContext(), 5*time.Second)
	defer cancel()

	v, err := h.service.GetVideo(ctx, id)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "video not found"})
	}
	return c.JSON(v)
}

func (h *Handler) Delete(c *fiber.Ctx) error {
	idStr := c.Params("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid id"})
	}

	ctx, cancel := context.WithTimeout(c.UserContext(), 5*time.Second)
	defer cancel()

	if _, err := h.service.GetVideo(ctx, id); err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "video not found"})
	}

	if err := h.service.DeleteVideo(ctx, id); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to delete video"})
	}
	return c.SendStatus(fiber.StatusNoContent)
}

func (h *Handler) IngestLocal(c *fiber.Ctx) error {
	var req struct {
		Directory string `json:"directory"`
		Path      string `json:"path"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
	}
	dir := req.Directory
	if dir == "" {
		dir = req.Path
	}
	if dir == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "directory is required"})
	}

	ctx, cancel := context.WithTimeout(c.UserContext(), 30*time.Second)
	defer cancel()

	count, err := h.service.IngestLocalDirectory(ctx, dir)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"ingested": count, "directory": dir})
}
