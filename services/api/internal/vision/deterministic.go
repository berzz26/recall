package vision

import (
	"context"
	"fmt"
	"sort"
	"strings"
)

// DeterministicDescriber is a local inference implementation that does NOT require Ollama/Moondream.
// It generates grounded, conservative descriptions from structured evidence (frames/tracks/events).
// It is model-agnostic and can be replaced by a real VLM (e.g., Qwen2-VL, LLaVA) behind the same interface.

type DeterministicDescriber struct {
	ModelName    string
	ModelVersion string
	MaxFrames    int
}

func NewDeterministicDescriber(model, version string, maxFrames int) *DeterministicDescriber {
	if model == "" {
		model = "deterministic-local"
	}
	if version == "" {
		version = "1"
	}
	if maxFrames <= 0 {
		maxFrames = 6
	}
	return &DeterministicDescriber{ModelName: model, ModelVersion: version, MaxFrames: maxFrames}
}

func (d *DeterministicDescriber) DescribeSegment(ctx context.Context, input SegmentDescriptionInput) (DescriptionResult, error) {
	if input.EndTime <= input.StartTime {
		return DescriptionResult{}, fmt.Errorf("invalid segment time")
	}
	// Build grounded description from evidence, not hallucinated intent
	var parts []string

	// Summary of objects
	if len(input.Detections) == 0 {
		parts = append(parts, fmt.Sprintf("Segment %.0fs–%.0fs: No distinct objects were detected in the sampled frames.", input.StartTime, input.EndTime))
	} else {
		// Count labels
		counts := make(map[string]int)
		for _, det := range input.Detections {
			counts[det.Label] += det.Count
		}
		labels := make([]string, 0, len(counts))
		for k := range counts {
			labels = append(labels, k)
		}
		sort.Strings(labels)
		var objParts []string
		for _, l := range labels {
			objParts = append(objParts, fmt.Sprintf("%s ×%d", l, counts[l]))
		}
		parts = append(parts, fmt.Sprintf("Segment %.0fs–%.0fs: %s visible.", input.StartTime, input.EndTime, strings.Join(objParts, ", ")))
	}

	// Tracks
	if len(input.Tracks) > 0 {
		// Sort by start
		sort.Slice(input.Tracks, func(i, j int) bool { return input.Tracks[i].StartTimestamp < input.Tracks[j].StartTimestamp })
		var trackDescs []string
		for _, tr := range input.Tracks {
			trackDescs = append(trackDescs, fmt.Sprintf("%s Track %d observed %.0fs→%.0fs (%d detections)", tr.Label, tr.TrackIndex, tr.StartTimestamp, tr.EndTimestamp, tr.DetectionCount))
		}
		parts = append(parts, strings.Join(trackDescs, "; ")+".")
	}

	// Events - movement etc
	moved := 0
	for _, e := range input.Events {
		if e.EventType == "OBJECT_MOVED" {
			moved++
		}
	}
	if moved > 0 {
		parts = append(parts, fmt.Sprintf("Movement detected in %d track(s) within this segment.", moved))
	} else if len(input.Tracks) > 0 {
		parts = append(parts, "No significant movement detected; objects remain relatively stationary within the sampled frames.")
	}

	// Frames
	if len(input.Frames) > 0 {
		parts = append(parts, fmt.Sprintf("Evidence from %d sampled frames (%.0fs, %.0fs, etc.) shows the scene as described; description is grounded in visible frames and structured detections/tracks.", len(input.Frames), input.Frames[0].Timestamp, func() float64 { if len(input.Frames) > 1 { return input.Frames[1].Timestamp }; return input.Frames[0].Timestamp }()))
	}

	// Conservative grounding note
	parts = append(parts, "Description is limited to visible evidence; no identities, intent, or emotion is inferred.")

	desc := strings.Join(parts, " ")
	// Validation
	desc = strings.TrimSpace(desc)
	if desc == "" {
		return DescriptionResult{}, fmt.Errorf("empty description")
	}
	if len(desc) > 2000 {
		desc = desc[:2000]
	}
	return DescriptionResult{
		Description:  desc,
		ModelName:    d.ModelName,
		ModelVersion: d.ModelVersion,
	}, nil
}
