package vision

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/berzz26/recall/services/api/internal/storage"
)

const (
	SmolVLMModelName    = "HuggingFaceTB/SmolVLM-500M-Instruct"
	SmolVLMModelVersion = "500M-Instruct"
)

type SmolVLMDescriber struct {
	pythonPath      string
	scriptPath      string
	modelName       string
	modelPath       string
	modelVersion    string
	maxFrames       int
	maxOutputTokens int
	timeout         time.Duration
	store           storage.Storage
}

func NewSmolVLMDescriber(pythonPath, scriptPath, modelName, modelVersion string, maxFrames int, store storage.Storage, timeout time.Duration) *SmolVLMDescriber {
	return NewSmolVLMDescriberWithConfig(pythonPath, scriptPath, modelName, "", modelVersion, maxFrames, 256, store, timeout)
}

func NewSmolVLMDescriberWithConfig(pythonPath, scriptPath, modelName, modelPath, modelVersion string, maxFrames, maxOutputTokens int, store storage.Storage, timeout time.Duration) *SmolVLMDescriber {
	if pythonPath == "" {
		pythonPath = "python3"
	}
	if modelName == "" {
		modelName = SmolVLMModelName
	}
	if modelVersion == "" {
		modelVersion = SmolVLMModelVersion
	}
	if maxFrames < 1 {
		maxFrames = 3
	}
	if maxOutputTokens < 1 {
		maxOutputTokens = 256
	}
	if timeout <= 0 {
		timeout = 10 * time.Minute
	}
	return &SmolVLMDescriber{
		pythonPath:      pythonPath,
		scriptPath:      scriptPath,
		modelName:       modelName,
		modelPath:       modelPath,
		modelVersion:    modelVersion,
		maxFrames:       maxFrames,
		maxOutputTokens: maxOutputTokens,
		timeout:         timeout,
		store:           store,
	}
}

func selectFrames(frames []FrameInput, max int) []FrameInput {
	if len(frames) == 0 {
		return nil
	}
	ordered := append([]FrameInput(nil), frames...)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].Timestamp < ordered[j].Timestamp })
	if len(ordered) <= max {
		return ordered
	}
	mid := (len(ordered) - 1) / 2
	return []FrameInput{ordered[0], ordered[mid], ordered[len(ordered)-1]}
}

func clamp(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

type qwenFrame struct {
	FrameID   string  `json:"frame_id"`
	Timestamp float64 `json:"timestamp_seconds"`
	Path      string  `json:"path"`
}

type qwenDetection struct {
	Label string `json:"label"`
	Count int    `json:"count"`
}

type qwenTrack struct {
	Label      string  `json:"label"`
	TrackIndex int     `json:"track_index"`
	Start      float64 `json:"start"`
	End        float64 `json:"end"`
}

type qwenEvent struct {
	EventType string   `json:"event_type"`
	Label     string   `json:"label"`
	Start     float64  `json:"start"`
	End       *float64 `json:"end,omitempty"`
}

type qwenSegment struct {
	SegmentID  string          `json:"segment_id"`
	StartTime  float64         `json:"start_time"`
	EndTime    float64         `json:"end_time"`
	Frames     []qwenFrame     `json:"frames"`
	Detections []qwenDetection `json:"detections"`
	Tracks     []qwenTrack     `json:"tracks"`
	Events     []qwenEvent     `json:"events"`
}

type qwenInput struct {
	VideoID  string        `json:"video_id"`
	Segments []qwenSegment `json:"segments"`
}

type qwenDescription struct {
	SegmentID    string `json:"segment_id"`
	Description  string `json:"description"`
	ModelName    string `json:"model_name"`
	ModelVersion string `json:"model_version"`
}

type qwenOutput struct {
	VideoID      string            `json:"video_id"`
	Descriptions []qwenDescription `json:"descriptions"`
}

func (q *SmolVLMDescriber) DescribeVideo(ctx context.Context, input VideoDescriptionInput) ([]DescriptionResult, error) {
	visionStart := time.Now()
	if len(input.Segments) == 0 {
		slog.Info("vision: empty segments, skipping", "video_id", input.VideoID.String(), "duration_ms", 0)
		return nil, nil
	}
	if _, err := os.Stat(q.scriptPath); err != nil {
		return nil, fmt.Errorf("vision script not found: %w", err)
	}

	tmpDir, err := os.MkdirTemp("", "recall-vision-*")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tmpDir)
	slog.Info("vision: start", "video_id", input.VideoID.String(), "segments", len(input.Segments), "max_frames_per_segment", q.maxFrames, "model", q.modelName, "model_version", q.modelVersion)

	var segs []qwenSegment
	for _, seg := range input.Segments {
		sel := selectFrames(seg.Frames, q.maxFrames)
		if len(sel) == 0 {
			return nil, fmt.Errorf("segment %s has no frames", seg.SegmentID)
		}
		var frames []qwenFrame
		for i, f := range sel {
			rc, err := q.store.Open(ctx, f.StorageKey)
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
			tmpPath := filepath.Join(tmpDir, fmt.Sprintf("%s-%04d.jpg", seg.SegmentID.String()[:8], i))
			if err := os.WriteFile(tmpPath, data, 0644); err != nil {
				return nil, err
			}
			frames = append(frames, qwenFrame{FrameID: f.ID.String(), Timestamp: f.Timestamp, Path: tmpPath})
		}

		// Compact detection context: aggregate counts by label.
		countByLabel := map[string]int{}
		for _, d := range seg.Detections {
			if d.Label == "" {
				continue
			}
			countByLabel[d.Label]++
		}
		var dets []qwenDetection
		for label, cnt := range countByLabel {
			dets = append(dets, qwenDetection{Label: label, Count: cnt})
		}
		sort.Slice(dets, func(i, j int) bool { return dets[i].Label < dets[j].Label })
		if dets == nil {
			dets = []qwenDetection{}
		}

		// Tracks overlapping the segment, clamped to segment boundaries.
		var tracks []qwenTrack
		for _, t := range seg.Tracks {
			if t.End < seg.StartTime || t.Start >= seg.EndTime {
				continue
			}
			tracks = append(tracks, qwenTrack{
				Label:      t.Label,
				TrackIndex: t.TrackIndex,
				Start:      clamp(t.Start, seg.StartTime, seg.EndTime),
				End:        clamp(t.End, seg.StartTime, seg.EndTime),
			})
		}
		sort.Slice(tracks, func(i, j int) bool {
			if tracks[i].Start == tracks[j].Start {
				return tracks[i].TrackIndex < tracks[j].TrackIndex
			}
			return tracks[i].Start < tracks[j].Start
		})
		if tracks == nil {
			tracks = []qwenTrack{}
		}

		// Events relevant to the segment, clamped.
		var events []qwenEvent
		for _, e := range seg.Events {
			end := e.End
			eEnd := e.Start
			if end != nil {
				eEnd = *end
			}
			if eEnd < seg.StartTime || e.Start >= seg.EndTime {
				continue
			}
			var clampedEnd *float64
			if end != nil {
				v := clamp(*end, seg.StartTime, seg.EndTime)
				clampedEnd = &v
			}
			events = append(events, qwenEvent{
				EventType: e.EventType,
				Label:     e.Label,
				Start:     clamp(e.Start, seg.StartTime, seg.EndTime),
				End:       clampedEnd,
			})
		}
		sort.Slice(events, func(i, j int) bool {
			if events[i].Start == events[j].Start {
				return events[i].EventType < events[j].EventType
			}
			return events[i].Start < events[j].Start
		})
		if events == nil {
			events = []qwenEvent{}
		}

		segs = append(segs, qwenSegment{
			SegmentID: seg.SegmentID.String(), StartTime: seg.StartTime, EndTime: seg.EndTime,
			Frames: frames, Detections: dets, Tracks: tracks, Events: events,
		})
	}

	prepStart := time.Now()
	req := qwenInput{VideoID: input.VideoID.String(), Segments: segs}
	raw, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	inputPath := filepath.Join(tmpDir, "vision-input.json")
	outputPath := filepath.Join(tmpDir, "vision-output.json")
	if err := os.WriteFile(inputPath, raw, 0644); err != nil {
		return nil, err
	}
	prepMs := time.Since(prepStart).Milliseconds()
	slog.Info("vision: input prepared", "video_id", input.VideoID.String(), "segments", len(segs), "duration_ms", prepMs)

	timeoutCtx, cancel := context.WithTimeout(ctx, q.timeout)
	defer cancel()
	cmd := exec.CommandContext(timeoutCtx, q.pythonPath, q.scriptPath, "--input", inputPath, "--output", outputPath)
	// Pass resolved configuration to Python so Go and Python cannot drift
	cmd.Env = append(os.Environ(),
		"VISION_MODEL="+q.modelName,
		"VISION_MODEL_PATH="+q.modelPath,
		"VISION_MODEL_VERSION="+q.modelVersion,
		"VISION_MAX_OUTPUT_TOKENS="+fmt.Sprint(q.maxOutputTokens),
	)
	var stderr bytes.Buffer
	visionLogWriter := &visionLogWriter{}
	cmd.Stderr = io.MultiWriter(&stderr, visionLogWriter)
	cmd.Stdout = io.MultiWriter(&stderr, visionLogWriter)
	subprocessStart := time.Now()
	slog.Info("vision: subprocess start", "video_id", input.VideoID.String(), "script", q.scriptPath, "timeout", q.timeout.String())
	if err := cmd.Run(); err != nil {
		subprocessMs := time.Since(subprocessStart).Milliseconds()
		msg := stderr.String()
		if len(msg) > 2000 {
			msg = msg[len(msg)-2000:]
		}
		if timeoutCtx.Err() == context.DeadlineExceeded {
			slog.Error("vision: subprocess timeout", "video_id", input.VideoID.String(), "duration_ms", subprocessMs, "error", timeoutCtx.Err())
			return nil, fmt.Errorf("smolvlm vision timeout: %w", timeoutCtx.Err())
		}
		slog.Error("vision: subprocess failed", "video_id", input.VideoID.String(), "duration_ms", subprocessMs, "stderr_tail", strings.TrimSpace(msg))
		trimmed := stderr.String()
		if len(trimmed) > 800 {
			trimmed = trimmed[:800]
		}
		return nil, fmt.Errorf("smolvlm vision failed: %s: %w", trimmed, err)
	}
	subprocessMs := time.Since(subprocessStart).Milliseconds()
	// Flush any remaining buffered line from streaming writer
	if trimmed := strings.TrimSpace(visionLogWriter.remaining); trimmed != "" {
		var js map[string]any
		if err := json.Unmarshal([]byte(trimmed), &js); err == nil {
			level, _ := js["level"].(string)
			msg, _ := js["msg"].(string)
			if msg == "" {
				msg = trimmed
			}
			delete(js, "msg")
			attrs := []any{}
			for k, v := range js {
				attrs = append(attrs, slog.Any(k, v))
			}
			switch strings.ToUpper(level) {
			case "ERROR":
				slog.Error("vision subprocess: "+msg, attrs...)
			case "WARN":
				slog.Warn("vision subprocess: "+msg, attrs...)
			default:
				slog.Info("vision subprocess: "+msg, attrs...)
			}
		} else {
			slog.Info("vision subprocess output", "output", trimmed)
		}
	}
	slog.Info("vision: subprocess complete", "video_id", input.VideoID.String(), "duration_ms", subprocessMs, "segments", len(segs))

	parseStart := time.Now()
	outData, err := os.ReadFile(outputPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read vision output: %w", err)
	}
	var out qwenOutput
	if err := json.Unmarshal(outData, &out); err != nil {
		return nil, fmt.Errorf("failed to parse vision output: %w", err)
	}
	parseMs := time.Since(parseStart).Milliseconds()
	slog.Info("vision: output parsed", "video_id", input.VideoID.String(), "descriptions", len(out.Descriptions), "duration_ms", parseMs)
	if out.VideoID != input.VideoID.String() {
		return nil, fmt.Errorf("vision output video_id mismatch")
	}
	bySegment := map[string]qwenDescription{}
	for _, d := range out.Descriptions {
		if d.SegmentID == "" || d.Description == "" {
			return nil, fmt.Errorf("invalid vision description entry")
		}
		if _, dup := bySegment[d.SegmentID]; dup {
			return nil, fmt.Errorf("duplicate vision description for segment %s", d.SegmentID)
		}
		bySegment[d.SegmentID] = d
	}

	var results []DescriptionResult
	for _, seg := range input.Segments {
		d, ok := bySegment[seg.SegmentID.String()]
		if !ok {
			return nil, fmt.Errorf("missing vision description for segment %s", seg.SegmentID)
		}
		if len(d.Description) > 2000 {
			d.Description = d.Description[:2000]
		}
		modelName := d.ModelName
		if modelName == "" {
			modelName = q.modelName
		}
		modelVersion := d.ModelVersion
		if modelVersion == "" {
			modelVersion = q.modelVersion
		}
		results = append(results, DescriptionResult{
			SegmentID: seg.SegmentID, Description: d.Description,
			ModelName: modelName, ModelVersion: modelVersion,
		})
	}
	totalMs := time.Since(visionStart).Milliseconds()
	slog.Info("vision: complete",
		"video_id", input.VideoID.String(),
		"segments", len(input.Segments),
		"descriptions", len(results),
		"prep_ms", prepMs,
		"subprocess_ms", subprocessMs,
		"parse_ms", parseMs,
		"total_duration_ms", totalMs,
	)
	return results, nil
}

// visionLogWriter streams VLM subprocess JSON logs line-by-line to slog
type visionLogWriter struct {
	remaining string
}

func (w *visionLogWriter) Write(p []byte) (int, error) {
	text := w.remaining + string(p)
	lines := strings.Split(text, "\n")
	w.remaining = lines[len(lines)-1]
	for _, line := range lines[:len(lines)-1] {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		var js map[string]any
		if err := json.Unmarshal([]byte(trimmed), &js); err == nil {
			level, _ := js["level"].(string)
			msg, _ := js["msg"].(string)
			if msg == "" {
				msg = trimmed
			}
			delete(js, "msg")
			attrs := []any{}
			for k, v := range js {
				attrs = append(attrs, slog.Any(k, v))
			}
			switch strings.ToUpper(level) {
			case "ERROR":
				slog.Error("vision subprocess: "+msg, attrs...)
			case "WARN":
				slog.Warn("vision subprocess: "+msg, attrs...)
			case "DEBUG":
				slog.Debug("vision subprocess: "+msg, attrs...)
			default:
				slog.Info("vision subprocess: "+msg, attrs...)
			}
		} else {
			slog.Info("vision subprocess output", "output", trimmed)
		}
	}
	return len(p), nil
}
