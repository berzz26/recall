package video_frame

import (
	"math"
	"testing"
	"time"

	"github.com/berzz26/recall/services/api/internal/video_segment"
	"github.com/google/uuid"
)

func TestSampleTimestamps10_2(t *testing.T) {
	ts, err := SampleTimestamps(10, 2*time.Second)
	if err != nil {
		t.Fatalf("err %v", err)
	}
	exp := []float64{0, 2, 4, 6, 8}
	if len(ts) != len(exp) {
		t.Fatalf("expected %d got %d %v", len(exp), len(ts), ts)
	}
	for i, e := range exp {
		if math.Abs(ts[i]-e) > 1e-9 {
			t.Fatalf("mismatch %d %v vs %v", i, ts[i], e)
		}
	}
}

func TestSampleShort(t *testing.T) {
	ts, err := SampleTimestamps(1, 2*time.Second)
	if err != nil {
		t.Fatalf("err %v", err)
	}
	if len(ts) != 1 || ts[0] != 0 {
		t.Fatalf("expected [0] got %v", ts)
	}
}

func TestSampleExactMultiple(t *testing.T) {
	ts, err := SampleTimestamps(10, 2*time.Second)
	if err != nil {
		t.Fatalf("err %v", err)
	}
	for _, v := range ts {
		if math.Abs(v-10) < 1e-9 {
			t.Fatalf("should not include duration 10")
		}
	}
}

func TestSampleFloating110(t *testing.T) {
	ts, err := SampleTimestamps(110.94, 2*time.Second)
	if err != nil {
		t.Fatalf("err %v", err)
	}
	if len(ts) != 56 {
		t.Fatalf("expected 56 got %d", len(ts))
	}
	last := ts[len(ts)-1]
	if last >= 110.94-1e-9 {
		t.Fatalf("last %v >= duration", last)
	}
	if math.Abs(last-110) > 1e-9 {
		t.Fatalf("last expected 110 got %v", last)
	}
}

func TestSampleInvalid(t *testing.T) {
	if _, err := SampleTimestamps(10, 0); err == nil {
		t.Fatalf("expected error for 0 interval")
	}
	if _, err := SampleTimestamps(0, 2*time.Second); err == nil {
		t.Fatalf("expected error for 0 duration")
	}
	if _, err := SampleTimestamps(-1, 2*time.Second); err == nil {
		t.Fatalf("expected error for negative")
	}
}

func TestFindSegment(t *testing.T) {
	vid := uuid.New()
	seg0 := video_segment.VideoSegment{ID: uuid.New(), VideoID: vid, SegmentIndex: 0, StartTime: 0, EndTime: 30, Duration: 30}
	seg1 := video_segment.VideoSegment{ID: uuid.New(), VideoID: vid, SegmentIndex: 1, StartTime: 30, EndTime: 60, Duration: 30}
	segs := []video_segment.VideoSegment{seg0, seg1}
	if got := FindSegment(segs, 29.999); got == nil || got.SegmentIndex != 0 {
		t.Fatalf("29.999 should be seg0")
	}
	if got := FindSegment(segs, 30.0); got == nil || got.SegmentIndex != 1 {
		t.Fatalf("30.0 should be seg1")
	}
	if got := FindSegment(segs, 0); got == nil || got.SegmentIndex != 0 {
		t.Fatalf("0 should be seg0")
	}
}

func TestSampleNoDuplicateDuration(t *testing.T) {
	ts, _ := SampleTimestamps(10, 2*time.Second)
	seen := map[float64]bool{}
	for _, v := range ts {
		if seen[v] {
			t.Fatalf("duplicate %v", v)
		}
		seen[v] = true
	}
}
