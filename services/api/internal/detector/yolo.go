package detector

import (
	"bufio"
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
	"sync"
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

// Session represents a persistent YOLO process that loads the model once per detection (per video)
// and handles multiple bounded batches without reloading. This eliminates per-batch model initialization overhead
// while preserving bounded memory via Go-side batch materialization.
type Session struct {
	y           *YoloDetector
	cmd         *exec.Cmd
	stdin       io.WriteCloser
	stdout      *bufio.Reader
	stderrPipe  io.ReadCloser
	logWriter   *detectorLogWriter
	mu          sync.Mutex
	closed      bool
	cancel      context.CancelFunc
	done        chan error
}

func (y *YoloDetector) NewSession(ctx context.Context) (*Session, error) {
	if _, err := os.Stat(y.scriptPath); err != nil {
		return nil, fmt.Errorf("detector script not found: %w", err)
	}
	args := []string{y.scriptPath, "--persistent", "--threshold", fmt.Sprintf("%f", y.threshold)}
	if y.modelPath != "" {
		args = append(args, "--model", y.modelPath)
	}
	// Use a cancellable context for the persistent process; it lives for the duration of the video detection.
	sessCtx, cancel := context.WithCancel(ctx)
	cmd := exec.CommandContext(sessCtx, y.pythonPath, args...)

	stdin, err := cmd.StdinPipe()
	if err != nil {
		cancel()
		return nil, fmt.Errorf("failed to create stdin pipe: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		return nil, fmt.Errorf("failed to create stdout pipe: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		cancel()
		return nil, fmt.Errorf("failed to create stderr pipe: %w", err)
	}

	// Stream stderr JSON logs via detectorLogWriter
	logWriter := &detectorLogWriter{}
	go func() {
		// Copy stderr line-by-line to slog; this runs for lifetime of process
		br := bufio.NewReader(stderr)
		for {
			line, err := br.ReadString('\n')
			if err != nil {
				if err != io.EOF {
					slog.Error("detector persistent stderr read error", "error", err)
				}
				return
			}
			_, _ = logWriter.Write([]byte(line))
		}
	}()

	if err := cmd.Start(); err != nil {
		cancel()
		return nil, fmt.Errorf("failed to start persistent detector: %w", err)
	}

	s := &Session{
		y:          y,
		cmd:        cmd,
		stdin:      stdin,
		stdout:     bufio.NewReader(stdout),
		stderrPipe: stderr,
		logWriter:  logWriter,
		cancel:     cancel,
		done:       make(chan error, 1),
	}
	go func() {
		s.done <- cmd.Wait()
	}()

	// Wait for ready signal (model loaded) with timeout. Model load can take up to ~60s.
	readyCtx, cancelReady := context.WithTimeout(ctx, 120*time.Second)
	defer cancelReady()
	readyCh := make(chan string, 1)
	errCh := make(chan error, 1)
	go func() {
		line, err := s.stdout.ReadString('\n')
		if err != nil {
			errCh <- err
			return
		}
		readyCh <- line
	}()
	select {
	case <-readyCtx.Done():
		_ = s.Close()
		return nil, fmt.Errorf("persistent detector startup timeout: %w", readyCtx.Err())
	case err := <-errCh:
		_ = s.Close()
		return nil, fmt.Errorf("persistent detector failed to start: %w", err)
	case line := <-readyCh:
		trimmed := strings.TrimSpace(line)
		var resp map[string]any
		if err := json.Unmarshal([]byte(trimmed), &resp); err != nil {
			// Not JSON - still consider ready if we got a line, but log
			slog.Warn("persistent detector ready signal not JSON", "line", trimmed)
		} else {
			if status, _ := resp["status"].(string); status != "ready" {
				// If first message is not ready, treat as ready anyway but log
				slog.Warn("persistent detector unexpected ready status", "status", status, "response", trimmed)
			}
			if ms, ok := resp["model_load_ms"]; ok {
				slog.Info("detection: persistent model loaded", "model", y.modelPath, "model_load_ms", ms)
			}
		}
		slog.Info("detection: persistent session ready", "script", y.scriptPath, "model", y.modelPath)
	}

	// Check if context already cancelled
	if ctx.Err() != nil {
		_ = s.Close()
		return nil, ctx.Err()
	}

	return s, nil
}

func (s *Session) AnalyzeBatch(ctx context.Context, frames []FrameInput) (map[uuid.UUID][]DetectionResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil, fmt.Errorf("session closed")
	}
	if len(frames) == 0 {
		return map[uuid.UUID][]DetectionResult{}, nil
	}
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	batchStart := time.Now()
	tmpDir, err := os.MkdirTemp("", "yolo-persist-*")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tmpDir)

	inputPath := filepath.Join(tmpDir, "input.json")
	outputPath := filepath.Join(tmpDir, "output.json")

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

	// Send request via stdin
	persistReq := map[string]any{
		"input":     inputPath,
		"output":    outputPath,
		"threshold": s.y.threshold,
	}
	line, _ := json.Marshal(persistReq)
	line = append(line, '\n')
	if _, err := s.stdin.Write(line); err != nil {
		return nil, fmt.Errorf("failed to send batch to persistent detector: %w", err)
	}

	// Wait for response line with context
	respCh := make(chan string, 1)
	errCh := make(chan error, 1)
	go func() {
		// Use a separate goroutine to read stdout line; this allows context cancellation
		respLine, err := s.stdout.ReadString('\n')
		if err != nil {
			errCh <- err
			return
		}
		respCh <- respLine
	}()

	var respLine string
	select {
	case <-ctx.Done():
		return nil, fmt.Errorf("yolo persistent batch cancelled: %w", ctx.Err())
	case err := <-errCh:
		// Check if process exited
		select {
		case procErr := <-s.done:
			return nil, fmt.Errorf("persistent detector process exited: %v (read error: %w)", procErr, err)
		default:
			return nil, fmt.Errorf("failed to read persistent detector response: %w", err)
		}
	case line := <-respCh:
		respLine = line
		// Check if process also exited unexpectedly
		select {
		case procErr := <-s.done:
			// Process exited but we got a line - log warning
			slog.Warn("persistent detector exited after response", "error", procErr)
		default:
		}
	}

	trimmed := strings.TrimSpace(respLine)
	var resp map[string]any
	if err := json.Unmarshal([]byte(trimmed), &resp); err != nil {
		return nil, fmt.Errorf("failed to parse persistent response %q: %w", trimmed, err)
	}
	status, _ := resp["status"].(string)
	if status != "ok" {
		errMsg, _ := resp["error"].(string)
		if errMsg == "" {
			errMsg = trimmed
		}
		if ctx.Err() == context.DeadlineExceeded {
			return nil, fmt.Errorf("yolo timeout: %w", ctx.Err())
		}
		return nil, fmt.Errorf("yolo persistent batch failed: %s", errMsg)
	}

	// Parse output file
	outData, err := os.ReadFile(outputPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read yolo output: %w", err)
	}
	var raw yoloResponse
	if err := json.Unmarshal(outData, &raw); err != nil {
		return nil, fmt.Errorf("failed to parse yolo output: %w", err)
	}
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
	// Ensure every input frame has an entry (persistent mode guarantees it, but be safe)
	for _, f := range frames {
		if _, ok := out[f.FrameID]; !ok {
			out[f.FrameID] = []DetectionResult{}
		}
	}
	totalMs := time.Since(batchStart).Milliseconds()
	slog.Info("detection: persistent batch complete",
		"frames", len(frames),
		"detections", totalDets,
		"total_duration_ms", totalMs,
		"avg_ms_per_frame", float64(totalMs)/float64(len(frames)),
	)
	return out, nil
}

func (s *Session) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil
	}
	s.closed = true
	// Send exit command
	_, _ = s.stdin.Write([]byte(`{"command":"exit"}` + "\n"))
	_ = s.stdin.Close()
	// Give process a moment to exit gracefully
	select {
	case err := <-s.done:
		s.cancel()
		if err != nil {
			slog.Warn("persistent detector exit error", "error", err)
		} else {
			slog.Info("detection: persistent session closed")
		}
		return err
	case <-time.After(5 * time.Second):
		s.cancel()
		// Force kill if not exited
		if s.cmd.Process != nil {
			_ = s.cmd.Process.Kill()
		}
		<-s.done
		slog.Warn("detection: persistent session killed after timeout")
		return fmt.Errorf("persistent detector close timeout")
	}
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
