package search

import (
	"context"
	"fmt"
	"strings"

	"github.com/berzz26/recall/services/api/internal/embedding"
	"github.com/berzz26/recall/services/api/internal/segment_embedding"
	"github.com/berzz26/recall/services/api/internal/video"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Service struct {
	embedder             embedding.TextEmbedder
	segmentEmbeddingRepo *segment_embedding.Repository
	db                   *pgxpool.Pool
	videoRepo            *video.Repository
	candidateLimit       int
	defaultLimit         int
	maxLimit             int
	minSimilarity        float64
}

func NewService(
	embedder embedding.TextEmbedder,
	segmentEmbeddingRepo *segment_embedding.Repository,
	db *pgxpool.Pool,
	videoRepo *video.Repository,
	candidateLimit, defaultLimit, maxLimit int,
	minSimilarity float64,
) *Service {
	return &Service{
		embedder:             embedder,
		segmentEmbeddingRepo: segmentEmbeddingRepo,
		db:                   db,
		videoRepo:            videoRepo,
		candidateLimit:       candidateLimit,
		defaultLimit:         defaultLimit,
		maxLimit:             maxLimit,
		minSimilarity:        minSimilarity,
	}
}

func (s *Service) Search(ctx context.Context, req SearchRequest) ([]SearchResult, error) {
	query := strings.TrimSpace(req.Query)
	if query == "" {
		return nil, fmt.Errorf("query is required")
	}
	limit := req.Limit
	if limit <= 0 {
		limit = s.defaultLimit
	}
	if limit < 1 || limit > s.maxLimit {
		return nil, fmt.Errorf("limit must be between 1 and %d", s.maxLimit)
	}
	if req.VideoID != nil {
		if *req.VideoID == uuid.Nil {
			return nil, fmt.Errorf("invalid video_id")
		}
	}
	// 1. Generate query embedding
	vec, err := s.embedder.EmbedQuery(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to embed query: %w", err)
	}
	if len(vec) != 384 {
		return nil, fmt.Errorf("invalid query embedding dimension %d", len(vec))
	}
	// 2. Retrieve candidates (bounded)
	candidates, err := s.segmentEmbeddingRepo.SearchSimilar(ctx, vec, s.candidateLimit, req.VideoID)
	if err != nil {
		return nil, fmt.Errorf("semantic retrieval failed: %w", err)
	}
	// 3. Apply threshold, preserve ranking (already descending similarity)
	var filtered []segment_embedding.SearchResult
	for _, c := range candidates {
		if c.Similarity >= s.minSimilarity {
			filtered = append(filtered, c)
		}
	}
	// 4. Enrich and truncate to requested limit
	var results []SearchResult
	for _, c := range filtered {
		if len(results) >= limit {
			break
		}
		r := SearchResult{
			VideoID:     c.Embedding.VideoID,
			SegmentID:   c.Embedding.SegmentID,
			StartTime:   c.StartTime,
			EndTime:     c.EndTime,
			Description: c.Description,
			Similarity:  c.Similarity,
			Detections:  []DetectionInfo{},
			Tracks:      []TrackInfo{},
			Events:      []EventInfo{},
		}
		// video filename enrichment
		if s.videoRepo != nil {
			v, err := s.videoRepo.GetByID(ctx, c.Embedding.VideoID)
			if err != nil {
				return nil, fmt.Errorf("video enrichment failed: %w", err)
			}
			r.Filename = v.Filename
			r.VideoName = v.Filename
		}
		// detections enrichment: distinct label, max confidence per label
		dets, err := s.enrichDetections(ctx, c.Embedding.VideoID, c.StartTime, c.EndTime)
		if err != nil {
			return nil, err
		}
		r.Detections = dets
		// tracks enrichment
		tracks, err := s.enrichTracks(ctx, c.Embedding.VideoID, c.StartTime, c.EndTime)
		if err != nil {
			return nil, err
		}
		r.Tracks = tracks
		// events enrichment
		events, err := s.enrichEvents(ctx, c.Embedding.VideoID, c.StartTime, c.EndTime)
		if err != nil {
			return nil, err
		}
		r.Events = events
		results = append(results, r)
	}
	if results == nil {
		results = []SearchResult{}
	}
	return results, nil
}

func (s *Service) enrichDetections(ctx context.Context, videoID uuid.UUID, segStart, segEnd float64) ([]DetectionInfo, error) {
	// Join frames to restrict to segment time range, then dedup by label max confidence
	query := `
		SELECT label, MAX(confidence) as max_conf
		FROM video_frame_detections d
		JOIN video_frames f ON f.id = d.frame_id
		WHERE d.video_id = $1
		  AND f.timestamp_seconds >= $2
		  AND f.timestamp_seconds < $3
		GROUP BY label
		ORDER BY label ASC
	`
	rows, err := s.db.Query(ctx, query, videoID, segStart, segEnd)
	if err != nil {
		return nil, fmt.Errorf("detection enrichment failed: %w", err)
	}
	defer rows.Close()
	var out []DetectionInfo
	for rows.Next() {
		var label string
		var conf float64
		if err := rows.Scan(&label, &conf); err != nil {
			return nil, fmt.Errorf("detection enrichment scan failed: %w", err)
		}
		out = append(out, DetectionInfo{Label: label, Confidence: conf})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("detection enrichment failed: %w", err)
	}
	if out == nil {
		out = []DetectionInfo{}
	}
	return out, nil
}

func (s *Service) enrichTracks(ctx context.Context, videoID uuid.UUID, segStart, segEnd float64) ([]TrackInfo, error) {
	query := `
		SELECT id, label, start_timestamp, end_timestamp
		FROM video_tracks
		WHERE video_id = $1
		  AND start_timestamp < $3
		  AND end_timestamp > $2
		ORDER BY start_timestamp ASC
	`
	rows, err := s.db.Query(ctx, query, videoID, segStart, segEnd)
	if err != nil {
		return nil, fmt.Errorf("track enrichment failed: %w", err)
	}
	defer rows.Close()
	var out []TrackInfo
	for rows.Next() {
		var id uuid.UUID
		var label string
		var st, et float64
		if err := rows.Scan(&id, &label, &st, &et); err != nil {
			return nil, fmt.Errorf("track enrichment scan failed: %w", err)
		}
		out = append(out, TrackInfo{TrackID: id, Label: label, StartTime: st, EndTime: et})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("track enrichment failed: %w", err)
	}
	if out == nil {
		out = []TrackInfo{}
	}
	return out, nil
}

func (s *Service) enrichEvents(ctx context.Context, videoID uuid.UUID, segStart, segEnd float64) ([]EventInfo, error) {
	query := `
		SELECT id, event_type, label, start_timestamp, end_timestamp, confidence
		FROM video_events
		WHERE video_id = $1
		  AND (
			(start_timestamp < $3 AND (end_timestamp IS NULL OR end_timestamp > $2))
			OR (start_timestamp >= $2 AND start_timestamp < $3)
		  )
		ORDER BY start_timestamp ASC
	`
	// Dedup by id (query already distinct per row)
	rows, err := s.db.Query(ctx, query, videoID, segStart, segEnd)
	if err != nil {
		return nil, fmt.Errorf("event enrichment failed: %w", err)
	}
	defer rows.Close()
	seen := make(map[uuid.UUID]bool)
	var out []EventInfo
	for rows.Next() {
		var id uuid.UUID
		var eventType, label string
		var st float64
		var et *float64
		var conf *float64
		if err := rows.Scan(&id, &eventType, &label, &st, &et, &conf); err != nil {
			return nil, fmt.Errorf("event enrichment scan failed: %w", err)
		}
		if seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, EventInfo{EventID: id, EventType: eventType, Label: label, StartTime: st, EndTime: et, Confidence: conf})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("event enrichment failed: %w", err)
	}
	if out == nil {
		out = []EventInfo{}
	}
	return out, nil
}
