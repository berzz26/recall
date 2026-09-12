package embedding

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/google/uuid"
)

const (
	DefaultModel        = "BAAI/bge-small-en-v1.5"
	DefaultModelVersion = "v1.5"
	QueryPrefix         = "Represent this sentence for searching relevant passages: "
)

// BGEEmbedder invokes workers/embedding/embed.py
type BGEEmbedder struct {
	pythonPath string
	scriptPath string
	timeout    time.Duration
}

func NewBGEEmbedder(pythonPath, scriptPath string, timeout time.Duration) *BGEEmbedder {
	if pythonPath == "" {
		pythonPath = "python3"
	}
	if scriptPath == "" {
		scriptPath = "workers/embedding/embed.py"
	}
	if timeout <= 0 {
		timeout = 5 * time.Minute
	}
	// Resolve script path similarly to detector
	if _, err := os.Stat(scriptPath); err != nil {
		if abs, err2 := filepath.Abs(scriptPath); err2 == nil {
			if _, err3 := os.Stat(abs); err3 == nil {
				scriptPath = abs
			}
		}
		if _, err := os.Stat(scriptPath); err != nil {
			alt := "/home/berzz/recall/workers/embedding/embed.py"
			if _, err2 := os.Stat(alt); err2 == nil {
				scriptPath = alt
			}
		}
	} else {
		if abs, err := filepath.Abs(scriptPath); err == nil {
			scriptPath = abs
		}
	}
	return &BGEEmbedder{pythonPath: pythonPath, scriptPath: scriptPath, timeout: timeout}
}

type embedInput struct {
	Items []embedInputItem `json:"items"`
	Mode  string           `json:"mode,omitempty"`
}

type embedInputItem struct {
	ID   string `json:"id"`
	Text string `json:"text"`
}

type embedOutput struct {
	Items []embedOutputItem `json:"items"`
}

type embedOutputItem struct {
	ID        string    `json:"id"`
	Embedding []float32 `json:"embedding"`
}

func (b *BGEEmbedder) embedBatch(ctx context.Context, items []embedInputItem, mode string) (map[string][]float32, error) {
	if len(items) == 0 {
		return map[string][]float32{}, nil
	}
	tmpDir, err := os.MkdirTemp("", "bge-*")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tmpDir)
	inputPath := filepath.Join(tmpDir, "input.json")
	outputPath := filepath.Join(tmpDir, "output.json")
	in := embedInput{Items: items, Mode: mode}
	data, _ := json.Marshal(in)
	if err := os.WriteFile(inputPath, data, 0644); err != nil {
		return nil, err
	}
	timeoutCtx, cancel := context.WithTimeout(ctx, b.timeout)
	defer cancel()
	cmd := exec.CommandContext(timeoutCtx, b.pythonPath, b.scriptPath, "--input", inputPath, "--output", outputPath, "--mode", mode)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	cmd.Stdout = &stderr
	if err := cmd.Run(); err != nil {
		msg := stderr.String()
		if len(msg) > 1000 {
			msg = msg[len(msg)-1000:]
		}
		if timeoutCtx.Err() == context.DeadlineExceeded {
			return nil, fmt.Errorf("embedding timeout: %w", timeoutCtx.Err())
		}
		return nil, fmt.Errorf("embedding failed: %s: %w", msg, err)
	}
	outData, err := os.ReadFile(outputPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read embedding output: %w", err)
	}
	var out embedOutput
	if err := json.Unmarshal(outData, &out); err != nil {
		return nil, fmt.Errorf("failed to parse embedding output: %w", err)
	}
	m := make(map[string][]float32, len(out.Items))
	for _, it := range out.Items {
		if len(it.Embedding) != 384 {
			return nil, fmt.Errorf("invalid embedding dimension for %s: %d != 384", it.ID, len(it.Embedding))
		}
		m[it.ID] = it.Embedding
	}
	return m, nil
}

func (b *BGEEmbedder) EmbedPassages(ctx context.Context, texts []string) ([][]float32, error) {
	items := make([]embedInputItem, len(texts))
	for i, t := range texts {
		if t == "" {
			return nil, fmt.Errorf("empty passage text at index %d", i)
		}
		items[i] = embedInputItem{ID: uuid.NewString(), Text: t}
	}
	m, err := b.embedBatch(ctx, items, "passage")
	if err != nil {
		return nil, err
	}
	out := make([][]float32, len(texts))
	for i, it := range items {
		v, ok := m[it.ID]
		if !ok {
			return nil, fmt.Errorf("missing embedding for %s", it.ID)
		}
		out[i] = v
	}
	return out, nil
}

func (b *BGEEmbedder) EmbedQuery(ctx context.Context, query string) ([]float32, error) {
	if query == "" {
		return nil, fmt.Errorf("empty query")
	}
	// BGE convention: prefix query
	id := uuid.NewString()
	items := []embedInputItem{{ID: id, Text: query}}
	// embed.py will add prefix when mode=query
	m, err := b.embedBatch(ctx, items, "query")
	if err != nil {
		return nil, err
	}
	v, ok := m[id]
	if !ok {
		return nil, fmt.Errorf("missing query embedding")
	}
	return v, nil
}

// EmbedPassagesWithIDs is helper for service layer to keep ID mapping.
func (b *BGEEmbedder) EmbedPassagesWithIDs(ctx context.Context, ids []string, texts []string) (map[string][]float32, error) {
	if len(ids) != len(texts) {
		return nil, fmt.Errorf("ids/texts length mismatch")
	}
	items := make([]embedInputItem, len(ids))
	for i := range ids {
		if texts[i] == "" {
			return nil, fmt.Errorf("empty text for id %s", ids[i])
		}
		items[i] = embedInputItem{ID: ids[i], Text: texts[i]}
	}
	return b.embedBatch(ctx, items, "passage")
}
