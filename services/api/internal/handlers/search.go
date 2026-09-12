package handlers

import (
	"context"
	"time"

	"github.com/berzz26/recall/services/api/internal/embedding"
	"github.com/berzz26/recall/services/api/internal/segment_embedding"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

type SearchHandler struct {
	embedder embedding.TextEmbedder
	repo     *segment_embedding.Repository
}

func NewSearchHandler(embedder embedding.TextEmbedder, repo *segment_embedding.Repository) *SearchHandler {
	return &SearchHandler{embedder: embedder, repo: repo}
}

type searchRequest struct {
	Query   string  `json:"query"`
	Limit   int     `json:"limit"`
	VideoID *string `json:"video_id,omitempty"`
}

type searchResponse struct {
	Query   string         `json:"query"`
	Results []searchResult `json:"results"`
}

type searchResult struct {
	VideoID     uuid.UUID `json:"video_id"`
	SegmentID   uuid.UUID `json:"segment_id"`
	StartTime   float64   `json:"start_time"`
	EndTime     float64   `json:"end_time"`
	Description string    `json:"description"`
	Similarity  float64   `json:"similarity"`
}

func (h *SearchHandler) Search(c *fiber.Ctx) error {
	var req searchRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
	}
	req.Query = trimSpace(req.Query)
	if req.Query == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "query is required"})
	}
	if req.Limit <= 0 {
		req.Limit = 10
	}
	if req.Limit < 1 || req.Limit > 50 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "limit must be between 1 and 50"})
	}
	var videoID *uuid.UUID
	if req.VideoID != nil && *req.VideoID != "" {
		vid, err := uuid.Parse(*req.VideoID)
		if err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid video_id"})
		}
		videoID = &vid
	}
	ctx, cancel := context.WithTimeout(c.UserContext(), 60*time.Second)
	defer cancel()
	vec, err := h.embedder.EmbedQuery(ctx, req.Query)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to embed query"})
	}
	if len(vec) != 384 {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "invalid query embedding dimension"})
	}
	results, err := h.repo.SearchSimilar(ctx, vec, req.Limit, videoID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "search failed"})
	}
	out := make([]searchResult, 0, len(results))
	for _, r := range results {
		out = append(out, searchResult{
			VideoID:     r.Embedding.VideoID,
			SegmentID:   r.Embedding.SegmentID,
			StartTime:   r.StartTime,
			EndTime:     r.EndTime,
			Description: r.Description,
			Similarity:  r.Similarity,
		})
	}
	return c.JSON(searchResponse{Query: req.Query, Results: out})
}

func trimSpace(s string) string {
	// avoid strings import overhead
	start := 0
	for start < len(s) && (s[start] == ' ' || s[start] == '\n' || s[start] == '\t' || s[start] == '\r') {
		start++
	}
	end := len(s)
	for end > start && (s[end-1] == ' ' || s[end-1] == '\n' || s[end-1] == '\t' || s[end-1] == '\r') {
		end--
	}
	return s[start:end]
}
