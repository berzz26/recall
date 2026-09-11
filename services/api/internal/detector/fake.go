package detector

import (
	"context"

	"github.com/google/uuid"
)

type FakeVisualAnalyzer struct {
	Results map[uuid.UUID][]DetectionResult
	Err     error
	Calls   int
}

func (f *FakeVisualAnalyzer) AnalyzeBatch(ctx context.Context, frames []FrameInput) (map[uuid.UUID][]DetectionResult, error) {
	f.Calls++
	if f.Err != nil {
		return nil, f.Err
	}
	if f.Results != nil {
		out := make(map[uuid.UUID][]DetectionResult, len(f.Results))
		for k, v := range f.Results {
			out[k] = v
		}
		return out, nil
	}
	out := make(map[uuid.UUID][]DetectionResult)
	for _, fr := range frames {
		out[fr.FrameID] = []DetectionResult{
			{Label: "person", Confidence: 0.94, BBoxX: 0.31, BBoxY: 0.12, BBoxWidth: 0.18, BBoxHeight: 0.65},
			{Label: "car", Confidence: 0.88, BBoxX: 0.62, BBoxY: 0.30, BBoxWidth: 0.29, BBoxHeight: 0.30},
		}
	}
	return out, nil
}

type SingleFakeAnalyzer struct {
	Fn func(FrameInput) []DetectionResult
}

func (s *SingleFakeAnalyzer) AnalyzeBatch(ctx context.Context, frames []FrameInput) (map[uuid.UUID][]DetectionResult, error) {
	out := make(map[uuid.UUID][]DetectionResult)
	for _, fr := range frames {
		out[fr.FrameID] = s.Fn(fr)
	}
	return out, nil
}
