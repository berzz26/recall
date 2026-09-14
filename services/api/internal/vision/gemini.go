package vision

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/berzz26/recall/services/api/internal/storage"
)

// Canonical Recall vision prompt — concise, temporally grounded semantic description optimized for retrieval.
const visionPrompt = `Generate a concise semantic description optimized for video retrieval for this CCTV video segment.

The supplied images are chronological representative frames from one continuous video segment.
You also receive bounded structured context: current detections/tracks/events for this segment, plus bounded temporal context (recent prior segment summaries, persistent tracks active before this segment, and recent prior events). This temporal context is CONTEXT FOR YOU to understand continuity — do NOT repeat it verbatim, do NOT list tracks/events, and do NOT expose track IDs, detector labels, confidence scores, or internal metadata in your answer. Use it only to determine whether an entity is continuing from a previous segment.

The images are the primary evidence. Structured detector, tracking, and event metadata is supplemental context only. It may be incomplete or incorrect. Do not make a claim merely because metadata says something exists. Verify visual claims against the images.

Describe in natural language as one concise paragraph, approximately 50-100 words, no bullets, no JSON, no frame-by-frame list. Return only the description.

Content to cover when visibly supported and useful for retrieval:
- Scene/context: what kind of environment/scene is visible, important objects/vehicles/people.
- Salient entities: describe visually meaningful people/objects when relevant, including useful visible attributes (clothing, color, carried objects, approximate location) and where they are in the scene.
- Activities/actions: what important entities are visibly doing, including meaningful interactions or actions.
- Temporal change: meaningful movement, appearance, disappearance, or state changes across the supplied frames.
- Cross-segment continuity: when tracking indicates the same entity continues from a previous segment AND the frames are visually consistent, use natural continuity language such as "the same person continues..." or "the person in the blue jacket moves..." Do NOT mention track IDs.

Rules:
- Describe only visible evidence.
- Prioritize salient entities and actions useful for semantic retrieval. Do NOT force every detected person/track into the paragraph.
- Do not identify real people or infer identities, names, intentions, or motives.
- Do not invent actions that are not visually supported and do not speculate about why something is happening.
- Use neutral language.
- If the scene is static, describe the visible scene concisely.
- Do not say that something is present if it is not visually supported.
- Do not mention detector confidence scores, track IDs, detector labels unless naturally useful, or internal implementation details.
- Do not mention that you are an AI.
- One paragraph, approximately 50-100 words. Maximum 100 words.

Return only the description.`

type GeminiDescriber struct {
	model           string
	modelVersion    string
	maxFrames       int
	maxOutputTokens int
	timeout         time.Duration
	apiKey          string
	store           storage.Storage
	httpClient      *http.Client
}

func NewGeminiDescriber(model, modelVersion string, maxFrames, maxOutputTokens int, timeout time.Duration, apiKey string, store storage.Storage) *GeminiDescriber {
	if maxFrames < 1 {
		maxFrames = 3
	}
	if maxOutputTokens < 1 {
		maxOutputTokens = 256
	}
	if timeout <= 0 {
		timeout = 10 * time.Minute
	}
	return &GeminiDescriber{
		model:           model,
		modelVersion:    modelVersion,
		maxFrames:       maxFrames,
		maxOutputTokens: maxOutputTokens,
		timeout:         timeout,
		apiKey:          apiKey,
		store:           store,
		httpClient:      &http.Client{},
	}
}

// buildVisionPrompt mirrors workers/vision/describe.py build_context.
func buildVisionPrompt(seg SegmentInput) string {
	var lines []string
	lines = append(lines, fmt.Sprintf("Current segment time: %.1fs to %.1fs (%.1fs duration)", seg.StartTime, seg.EndTime, seg.EndTime-seg.StartTime))
	if len(seg.Detections) > 0 {
		labelSet := map[string]struct{}{}
		for _, d := range seg.Detections {
			if d.Label != "" {
				labelSet[d.Label] = struct{}{}
			}
		}
		if len(labelSet) > 0 {
			var labels []string
			for l := range labelSet {
				labels = append(labels, l)
			}
			// sort for determinism
			// simple sort
			for i := 0; i < len(labels); i++ {
				for j := i + 1; j < len(labels); j++ {
					if labels[j] < labels[i] {
						labels[i], labels[j] = labels[j], labels[i]
					}
				}
			}
			lines = append(lines, "Visible detector labels in this segment:\n"+strings.Join(labels, ", "))
		}
	} else {
		lines = append(lines, "Visible detector labels in this segment:\nnone reported")
	}
	if len(seg.Tracks) > 0 {
		lines = append(lines, "Tracks overlapping this segment (label, absolute start, absolute end, duration, continuity):")
		for _, t := range seg.Tracks {
			continuity := "new in this segment"
			if seg.History != nil {
				for _, pt := range seg.History.PersistentTracks {
					if pt.TrackIndex == t.TrackIndex && pt.Label == t.Label && pt.Start < seg.StartTime {
						continuity = fmt.Sprintf("continuing from %.1fs", pt.Start)
						break
					}
				}
			}
			dur := t.End - t.Start
			lines = append(lines, fmt.Sprintf("- %s track %d, %.1fs to %.1fs (%.1fs), %s", t.Label, t.TrackIndex, t.Start, t.End, dur, continuity))
		}
	} else {
		lines = append(lines, "Tracks overlapping this segment: none")
	}
	if len(seg.Events) > 0 {
		lines = append(lines, "Events in this segment (type, label, absolute start, end):")
		for _, e := range seg.Events {
			endS := "none"
			if e.End != nil {
				endS = fmt.Sprintf("%.1fs", *e.End)
			}
			lines = append(lines, fmt.Sprintf("- %s, %s, %.1fs to %s", e.EventType, e.Label, e.Start, endS))
		}
	} else {
		lines = append(lines, "Events in this segment: none")
	}
	// Temporal history: prior segments, persistent tracks, prior events
	if seg.History != nil {
		if len(seg.History.PriorSegments) > 0 {
			lines = append(lines, fmt.Sprintf("Temporal history: %d prior segment(s) before %.1fs:", len(seg.History.PriorSegments), seg.StartTime))
			for _, ps := range seg.History.PriorSegments {
				lab := "none"
				if len(ps.Labels) > 0 {
					lab = strings.Join(ps.Labels, ", ")
				}
				lines = append(lines, fmt.Sprintf("- Prior segment %d (%.1fs-%.1fs): labels [%s], %d track(s), %d event(s)", ps.SegmentIndex, ps.StartTime, ps.EndTime, lab, len(ps.Tracks), len(ps.Events)))
				for _, pt := range ps.Tracks {
					lines = append(lines, fmt.Sprintf("  * track %s %d: %.1fs-%.1fs", pt.Label, pt.TrackIndex, pt.Start, pt.End))
				}
				for _, pe := range ps.Events {
					endS := "none"
					if pe.End != nil {
						endS = fmt.Sprintf("%.1fs", *pe.End)
					}
					lines = append(lines, fmt.Sprintf("  * event %s %s: %.1fs to %s", pe.EventType, pe.Label, pe.Start, endS))
				}
			}
		} else {
			lines = append(lines, "Temporal history: no prior segments (this is the first segment)")
		}
		if len(seg.History.PersistentTracks) > 0 {
			lines = append(lines, fmt.Sprintf("Persistent tracks active before this segment (%d):", len(seg.History.PersistentTracks)))
			for _, pt := range seg.History.PersistentTracks {
				lines = append(lines, fmt.Sprintf("- %s track %d, %.1fs to %.1fs (%.1fs)", pt.Label, pt.TrackIndex, pt.Start, pt.End, pt.End-pt.Start))
			}
		} else {
			lines = append(lines, "Persistent tracks before this segment: none")
		}
		if len(seg.History.PriorEvents) > 0 {
			lines = append(lines, fmt.Sprintf("Prior events before this segment (%d):", len(seg.History.PriorEvents)))
			for _, pe := range seg.History.PriorEvents {
				endS := "none"
				if pe.End != nil {
					endS = fmt.Sprintf("%.1fs", *pe.End)
				}
				lines = append(lines, fmt.Sprintf("- %s %s: %.1fs to %s", pe.EventType, pe.Label, pe.Start, endS))
			}
		} else {
			lines = append(lines, "Prior events before this segment: none")
		}
	}
	return visionPrompt + "\n\nStructured context (compact):\n" + strings.Join(lines, "\n")
}

type geminiPart struct {
	Text       *string           `json:"text,omitempty"`
	InlineData *geminiInlineData `json:"inline_data,omitempty"`
}

type geminiInlineData struct {
	MimeType string `json:"mime_type"`
	Data     string `json:"data"`
}

type geminiContent struct {
	Role  string       `json:"role"`
	Parts []geminiPart `json:"parts"`
}

type geminiRequest struct {
	Contents         []geminiContent        `json:"contents"`
	GenerationConfig map[string]interface{} `json:"generationConfig,omitempty"`
}

type geminiResponse struct {
	Candidates []struct {
		Content struct {
			Parts []struct {
				Text string `json:"text"`
			} `json:"parts"`
		} `json:"content"`
		FinishReason string `json:"finishReason"`
	} `json:"candidates"`
	Error *struct {
		Message string `json:"message"`
		Code    int    `json:"code"`
	} `json:"error"`
}

func (g *GeminiDescriber) DescribeVideo(ctx context.Context, input VideoDescriptionInput) ([]DescriptionResult, error) {
	start := time.Now()
	if len(input.Segments) == 0 {
		return nil, nil
	}
	if g.apiKey == "" {
		return nil, fmt.Errorf("GEMINI_API_KEY is required when VISION_PROVIDER=gemini")
	}
	slog.Info("vision: gemini start", "video_id", input.VideoID.String(), "segments", len(input.Segments), "model", g.model, "max_frames", g.maxFrames)

	var results []DescriptionResult
	for idx, seg := range input.Segments {
		sel := selectFrames(seg.Frames, g.maxFrames)
		if len(sel) == 0 {
			return nil, fmt.Errorf("segment %s has no frames", seg.SegmentID)
		}
		segStart := time.Now()
		slog.Info("vision: gemini segment start", "video_id", input.VideoID.String(), "segment_index", idx+1, "segment_id", seg.SegmentID.String(), "frames", len(sel))

		prompt := buildVisionPrompt(seg)

		var parts []geminiPart
		for _, f := range sel {
			rc, err := g.store.Open(ctx, f.StorageKey)
			if err != nil {
				return nil, fmt.Errorf("failed to open frame %s: %w", f.ID, err)
			}
			data, err := io.ReadAll(rc)
			rc.Close()
			if err != nil {
				return nil, fmt.Errorf("failed to read frame %s: %w", f.ID, err)
			}
			if len(data) == 0 {
				return nil, fmt.Errorf("empty frame %s", f.ID)
			}
			b64 := base64.StdEncoding.EncodeToString(data)
			mime := "image/jpeg"
			parts = append(parts, geminiPart{InlineData: &geminiInlineData{MimeType: mime, Data: b64}})
		}
		text := prompt
		parts = append(parts, geminiPart{Text: &text})

		reqBody := geminiRequest{
			Contents: []geminiContent{{Role: "user", Parts: parts}},
			GenerationConfig: map[string]interface{}{
				"maxOutputTokens": g.maxOutputTokens,
				"temperature":     0.2,
			},
		}
		bodyBytes, err := json.Marshal(reqBody)
		if err != nil {
			return nil, err
		}

		url := fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models/%s:generateContent", g.model)
		httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(bodyBytes))
		if err != nil {
			return nil, err
		}
		httpReq.Header.Set("Content-Type", "application/json")
		httpReq.Header.Set("X-Goog-Api-Key", g.apiKey)

		// Use timeout via context; create per-request timeout derived from parent
		reqCtx, cancel := context.WithTimeout(ctx, g.timeout)
		httpReq = httpReq.WithContext(reqCtx)

		resp, err := g.httpClient.Do(httpReq)
		if err != nil {
			// Check timeout before cancel
			if reqCtx.Err() == context.DeadlineExceeded {
				cancel()
				return nil, fmt.Errorf("gemini timeout for segment %s: %w", seg.SegmentID, err)
			}
			cancel()
			// Also handle generic context canceled that wraps timeout
			if ctx.Err() != nil {
				return nil, fmt.Errorf("gemini request canceled for segment %s: %w", seg.SegmentID, ctx.Err())
			}
			return nil, fmt.Errorf("gemini request failed for segment %s: %w", seg.SegmentID, err)
		}
		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		cancel()
		if err != nil {
			if reqCtx.Err() == context.DeadlineExceeded {
				return nil, fmt.Errorf("gemini timeout while reading response for segment %s: %w", seg.SegmentID, err)
			}
			return nil, fmt.Errorf("failed to read gemini response: %w", err)
		}
		if resp.StatusCode == 429 {
			return results, fmt.Errorf("%w: gemini rate limited for segment %s: HTTP 429 %s", ErrRateLimited, seg.SegmentID, string(body))
		}
		if resp.StatusCode == 401 || resp.StatusCode == 403 {
			return nil, fmt.Errorf("gemini authentication failed for segment %s: HTTP %d", seg.SegmentID, resp.StatusCode)
		}
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return nil, fmt.Errorf("gemini HTTP error for segment %s: HTTP %d %s", seg.SegmentID, resp.StatusCode, string(body))
		}
		var gemResp geminiResponse
		if err := json.Unmarshal(body, &gemResp); err != nil {
			return nil, fmt.Errorf("malformed gemini response for segment %s: %w", seg.SegmentID, err)
		}
		if gemResp.Error != nil {
			return nil, fmt.Errorf("gemini error for segment %s: %s", seg.SegmentID, gemResp.Error.Message)
		}
		if len(gemResp.Candidates) == 0 || len(gemResp.Candidates[0].Content.Parts) == 0 {
			return nil, fmt.Errorf("empty gemini output for segment %s", seg.SegmentID)
		}
		var sb strings.Builder
		for _, p := range gemResp.Candidates[0].Content.Parts {
			sb.WriteString(p.Text)
		}
		desc := strings.TrimSpace(sb.String())
		if desc == "" {
			return nil, fmt.Errorf("empty gemini output for segment %s", seg.SegmentID)
		}
		slog.Info("vision: gemini segment complete", "video_id", input.VideoID.String(), "segment_id", seg.SegmentID.String(), "description_length", len(desc), "duration_ms", time.Since(segStart).Milliseconds())
		results = append(results, DescriptionResult{
			SegmentID: seg.SegmentID, Description: desc, ModelName: g.model, ModelVersion: g.modelVersion,
		})
	}
	slog.Info("vision: gemini complete", "video_id", input.VideoID.String(), "segments", len(results), "total_duration_ms", time.Since(start).Milliseconds())
	return results, nil
}
