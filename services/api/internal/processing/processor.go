package processing

import (
	"context"

	"github.com/berzz26/recall/services/api/internal/video"
)

type Processor interface {
	Process(ctx context.Context, v *video.Video) error
}

type NoopProcessor struct{}

func (p *NoopProcessor) Process(ctx context.Context, v *video.Video) error {
	return nil
}

type FailingProcessor struct {
	Err error
}

func (p *FailingProcessor) Process(ctx context.Context, v *video.Video) error {
	if p.Err != nil {
		return p.Err
	}
	return &testError{"failing processor"}
}

type testError struct{ msg string }

func (e *testError) Error() string {
	if e.msg != "" {
		return e.msg
	}
	return "processing failed"
}
