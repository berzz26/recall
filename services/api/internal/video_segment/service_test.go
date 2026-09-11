package video_segment

import (
	"math"
	"testing"
	"time"

	"github.com/google/uuid"
)

func floatEq(a, b float64) bool {
	return math.Abs(a-b) < 1e-9
}

func TestBuildSegments110(t *testing.T) {
	id := uuid.New()
	segs, err := BuildSegments(id, 110.94, 30*time.Second)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(segs) != 4 {
		t.Fatalf("expected 4 got %d", len(segs))
	}
	exp := [][2]float64{{0, 30}, {30, 60}, {60, 90}, {90, 110.94}}
	for i, e := range exp {
		if !floatEq(segs[i].StartTime, e[0]) || !floatEq(segs[i].EndTime, e[1]) {
			t.Fatalf("seg %d expected %v got %v %v", i, e, segs[i].StartTime, segs[i].EndTime)
		}
		if !floatEq(segs[i].Duration, e[1]-e[0]) {
			t.Fatalf("duration mismatch %d", i)
		}
		if segs[i].SegmentIndex != i {
			t.Fatalf("index mismatch")
		}
		if !(0 <= segs[i].StartTime && segs[i].StartTime < segs[i].EndTime && segs[i].EndTime <= 110.94+1e-9) {
			t.Fatalf("invariant failed %d", i)
		}
	}
}

func TestBuildSegmentsExactMultiple(t *testing.T) {
	id := uuid.New()
	segs, err := BuildSegments(id, 60, 30*time.Second)
	if err != nil {
		t.Fatalf("err %v", err)
	}
	if len(segs) != 2 {
		t.Fatalf("expected 2 got %d", len(segs))
	}
	if !floatEq(segs[0].StartTime, 0) || !floatEq(segs[0].EndTime, 30) {
		t.Fatalf("seg0 mismatch")
	}
	if !floatEq(segs[1].StartTime, 30) || !floatEq(segs[1].EndTime, 60) {
		t.Fatalf("seg1 mismatch")
	}
}

func TestBuildSegmentsShort(t *testing.T) {
	id := uuid.New()
	segs, err := BuildSegments(id, 10, 30*time.Second)
	if err != nil {
		t.Fatalf("err %v", err)
	}
	if len(segs) != 1 {
		t.Fatalf("expected 1 got %d", len(segs))
	}
	if !floatEq(segs[0].StartTime, 0) || !floatEq(segs[0].EndTime, 10) {
		t.Fatalf("mismatch")
	}
}

func TestBuildSegmentsVerySmall(t *testing.T) {
	id := uuid.New()
	segs, err := BuildSegments(id, 0.5, 30*time.Second)
	if err != nil {
		t.Fatalf("err %v", err)
	}
	if len(segs) != 1 {
		t.Fatalf("expected 1 got %d", len(segs))
	}
	if !floatEq(segs[0].EndTime, 0.5) {
		t.Fatalf("mismatch")
	}
}

func TestBuildSegmentsInvalidDuration(t *testing.T) {
	id := uuid.New()
	for _, d := range []float64{0, -1, -0.1} {
		if _, err := BuildSegments(id, d, 30*time.Second); err == nil {
			t.Fatalf("expected error for duration %v", d)
		}
	}
}

func TestBuildSegmentsInvalidSegmentDuration(t *testing.T) {
	id := uuid.New()
	for _, d := range []time.Duration{0, -5 * time.Second} {
		if _, err := BuildSegments(id, 10, d); err == nil {
			t.Fatalf("expected error for seg dur %v", d)
		}
	}
}

func TestBuildSegmentsNoZeroLengthFinal(t *testing.T) {
	id := uuid.New()
	segs, err := BuildSegments(id, 60, 30*time.Second)
	if err != nil {
		t.Fatalf("err %v", err)
	}
	if len(segs) != 2 {
		t.Fatalf("expected 2 not 3")
	}
	for _, s := range segs {
		if s.Duration <= 0 {
			t.Fatalf("zero length")
		}
	}
}

func TestBuildSegmentsFloatingTolerance(t *testing.T) {
	id := uuid.New()
	segs, err := BuildSegments(id, 110.94, 30*time.Second)
	if err != nil {
		t.Fatalf("err %v", err)
	}
	last := segs[len(segs)-1]
	if !floatEq(last.EndTime, 110.94) {
		t.Fatalf("final not clamped %v", last.EndTime)
	}
}

func TestBuildSegmentsInvariants(t *testing.T) {
	id := uuid.New()
	cases := []struct {
		dur float64
		seg time.Duration
	}{
		{110.94, 30 * time.Second},
		{60, 30 * time.Second},
		{10, 30 * time.Second},
		{100, 10 * time.Second},
		{65, 30 * time.Second},
	}
	for _, c := range cases {
		segs, err := BuildSegments(id, c.dur, c.seg)
		if err != nil {
			t.Fatalf("err %v", err)
		}
		for i, s := range segs {
			if s.StartTime < -1e-9 || s.EndTime > c.dur+1e-9 || s.StartTime >= s.EndTime {
				t.Fatalf("invariant failed i=%d %+v", i, s)
			}
			if !floatEq(s.Duration, s.EndTime-s.StartTime) {
				t.Fatalf("duration mismatch")
			}
			if i > 0 && !floatEq(s.StartTime, segs[i-1].EndTime) {
				t.Fatalf("non contiguous")
			}
		}
	}
}
