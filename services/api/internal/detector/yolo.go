package detector

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

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
	if len(frames) == 0 {
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

	args := []string{y.scriptPath, "--input", inputPath, "--output", outputPath, "--threshold", fmt.Sprintf("%f", y.threshold)}
	if y.modelPath != "" {
		args = append(args, "--model", y.modelPath)
	}
	cmd := exec.CommandContext(ctx, y.pythonPath, args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	cmd.Stdout = &stderr
	if err := cmd.Run(); err != nil {
		msg := stderr.String()
		if len(msg) > 800 {
			msg = msg[:800]
		}
		if ctx.Err() == context.DeadlineExceeded {
			return nil, fmt.Errorf("yolo timeout: %w", ctx.Err())
		}
		return nil, fmt.Errorf("yolo failed: %s: %w", msg, err)
	}
	outData, err := os.ReadFile(outputPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read yolo output: %w", err)
	}
	var raw yoloResponse
	if err := json.Unmarshal(outData, &raw); err != nil {
		return nil, fmt.Errorf("failed to parse yolo output: %w", err)
	}
	out := make(map[uuid.UUID][]DetectionResult, len(raw))
	for k, v := range raw {
		id, err := uuid.Parse(k)
		if err != nil {
			continue
		}
		out[id] = v
	}
	return out, nil
}
