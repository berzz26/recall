package embedding

import (
	"context"
)

// TextEmbedder provides BGE embedding for passages and queries.
type TextEmbedder interface {
	EmbedPassages(ctx context.Context, texts []string) ([][]float32, error)
	EmbedQuery(ctx context.Context, query string) ([]float32, error)
}
