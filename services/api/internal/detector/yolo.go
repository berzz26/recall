package detector

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
	"strings"
	"time"

	"github.com/google/uuid"
)

type YoloDetector struct {
	pythonPath string
	scriptPath string
	modelPath  string
	threshold  float64
}

func NewYoloDetector(pythonPath, scriptPath, modelPath string, threshold float64) *YoloDetector {
	if pythonPath == "" {
		pythonPath = "python3"
	}
	if threshold < 0 || threshold > 1 {
		threshold = 0.25
	}
	return &YoloDetector{pythonPath: pythonPath, scriptPath: scriptPath, modelPath: modelPath, threshold: threshold}
}

type yoloRequest struct {
	Frames []yoloFrame `json:"frames"`
}

type yoloFrame struct {
	FrameID string `json:"frame_id"`
	Path    string `json:"path"`
	Width   int    `json:"width"`
	Height  int    `json:"height"`
}

type yoloResponse map[string][]DetectionResult

func (y *YoloDetector) AnalyzeBatch(ctx context.Context, frames []FrameInput) (map[uuid.UUID][]DetectionResult, error) {
	batchStart := time.Now()
	if len(frames) == 0 {
		slog.Info("detection: empty batch, skipping", "frames", 0, "duration_ms", 0)
		return map[uuid.UUID][]DetectionResult{}, nil
	}
	if _, err := os.Stat(y.scriptPath); err != nil {
		return nil, fmt.Errorf("detector script not found: %w", err)
	}
	tmpDir, err := os.MkdirTemp("", "yolo-req-*")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tmpDir)

	inputPath := filepath.Join(tmpDir, "input.json")
	outputPath := filepath.Join(tmpDir, "output.json")

	prepStart := time.Now()
	var req yoloRequest
	for _, f := range frames {
		if f.LocalPath == "" {
			return nil, fmt.Errorf("frame %s missing local path", f.FrameID)
		}
		req.Frames = append(req.Frames, yoloFrame{FrameID: f.FrameID.String(), Path: f.LocalPath, Width: f.Width, Height: f.Height})
	}
	data, _ := json.Marshal(req)
	if err := os.WriteFile(inputPath, data, 0644); err != nil {
		return nil, err
	}
	prepMs := time.Since(prepStart).Milliseconds()
	slog.Info("detection: input prepared", "frames", len(frames), "duration_ms", prepMs, "threshold", y.threshold, "model", y.modelPath)

	args := []string{y.scriptPath, "--input", inputPath, "--output", outputPath, "--threshold", fmt.Sprintf("%f", y.threshold)}
	if y.modelPath != "" {
		args = append(args, "--model", y.modelPath)
	}
	cmd := exec.CommandContext(ctx, y.pythonPath, args...)
	var stderr bytes.Buffer
	// Stream subprocess JSON logs to terminal (production-grade) while capturing for error reporting
	streamWriter := &detectorLogWriter{}
	cmd.Stderr = io.MultiWriter(&stderr, streamWriter)
	cmd.Stdout = io.MultiWriter(&stderr, streamWriter)
	subprocessStart := time.Now()
	slog.Info("detection: subprocess start", "frames", len(frames), "script", y.scriptPath, "threshold", y.threshold)
	if err := cmd.Run(); err != nil {
		subprocessMs := time.Since(subprocessStart).Milliseconds()
		msg := stderr.String()
		// Also ensure buffered logs are flushed to terminal (already via MultiWriter)
		if len(msg) > 2000 {
			msg = msg[:2000]
		}
		// Try to extract last JSON line for context
		if ctx.Err() == context.DeadlineExceeded {
			slog.Error("detection: subprocess timeout", "frames", len(frames), "duration_ms", subprocessMs, "error", ctx.Err())
			return nil, fmt.Errorf("yolo timeout: %w", ctx.Err())
		}
		slog.Error("detection: subprocess failed", "frames", len(frames), "duration_ms", subprocessMs, "stderr_tail", strings.TrimSpace(msg[len(msg)-500:]))
		return nil, fmt.Errorf("yolo failed: %s: %w", msg, err)
	}
	subprocessMs := time.Since(subprocessStart).Milliseconds()
	if trimmed := strings.TrimSpace(streamWriter.remaining); trimmed != "" {
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
				slog.Error("detector subprocess: "+msg, attrs...)
			case "WARN":
				slog.Warn("detector subprocess: "+msg, attrs...)
			default:
				slog.Info("detector subprocess: "+msg, attrs...)
			}
		} else {
			slog.Info("detector subprocess output", "output", trimmed)
		}
	}
	slog.Info("detection: subprocess complete", "frames", len(frames), "duration_ms", subprocessMs)

	parseStart := time.Now()
	outData, err := os.ReadFile(outputPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read yolo output: %w", err)
	}
	var raw yoloResponse
	if err := json.Unmarshal(outData, &raw); err != nil {
		return nil, fmt.Errorf("failed to parse yolo output: %w", err)
	}
	parseMs := time.Since(parseStart).Milliseconds()
	out := make(map[uuid.UUID][]DetectionResult, len(raw))
	totalDets := 0
	for k, v := range raw {
		id, err := uuid.Parse(k)
		if err != nil {
			continue
		}
		out[id] = v
		totalDets += len(v)
	}
	totalMs := time.Since(batchStart).Milliseconds()
	slog.Info("detection: batch complete",
		"frames", len(frames),
		"detections", totalDets,
		"prep_ms", prepMs,
		"subprocess_ms", subprocessMs,
		"parse_ms", parseMs,
		"total_duration_ms", totalMs,
		"avg_ms_per_frame", float64(totalMs)/float64(len(frames)),
	)
	return out, nil
}

// detectorLogWriter streams subprocess output line-by-line to slog while buffering
type detectorLogWriter struct {
	remaining string
}

func (w *detectorLogWriter) Write(p []byte) (int, error) {
	// Buffer already handled via MultiWriter's &w.buf, but we also log lines
	text := w.remaining + string(p)
	lines := strings.Split(text, "\n")
	w.remaining = lines[len(lines)-1]
	for _, line := range lines[:len(lines)-1] {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		// If line is JSON (production-grade), log as structured; otherwise plain
		var js map[string]any
		if err := json.Unmarshal([]byte(trimmed), &js); err == nil {
			// Extract level and msg for slog level routing
			level, _ := js["level"].(string)
			msg, _ := js["msg"].(string)
			if msg == "" {
				msg = trimmed
			}
			// Remove duplicate fields to avoid clutter
			delete(js, "msg")
			// Keep component/timestamp for context but log via slog
			attrs := []any{}
			for k, v := range js {
				attrs = append(attrs, slog.Any(k, v))
			}
			switch strings.ToUpper(level) {
			case "ERROR":
				slog.Error("detector subprocess: "+msg, attrs...)
			case "WARN":
				slog.Warn("detector subprocess: "+msg, attrs...)
			default:
				slog.Info("detector subprocess: "+msg, attrs...)
			}
		} else {
			slog.Info("detector subprocess output", "output", trimmed)
		}
	}
	return len(p), nil
}
