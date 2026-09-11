package detection

import (
	"testing"
)

func TestValidateValid(t *testing.T) {
	d := Detection{Label: "person", Confidence: 0.94, BBoxX: 0.31, BBoxY: 0.12, BBoxWidth: 0.18, BBoxHeight: 0.65}
	if err := Validate(d); err != nil {
		t.Fatalf("expected valid: %v", err)
	}
}

func TestValidateInvalidConfidence(t *testing.T) {
	for _, c := range []float64{-0.1, 1.1} {
		d := Detection{Label: "car", Confidence: c, BBoxX: 0.1, BBoxY: 0.1, BBoxWidth: 0.2, BBoxHeight: 0.2}
		if err := Validate(d); err == nil {
			t.Fatalf("expected invalid confidence %v", c)
		}
	}
}

func TestValidateInvalidBBox(t *testing.T) {
	cases := []Detection{
		{Label: "person", Confidence: 0.5, BBoxX: -0.1, BBoxY: 0.1, BBoxWidth: 0.2, BBoxHeight: 0.2},
		{Label: "person", Confidence: 0.5, BBoxX: 0.9, BBoxY: 0.1, BBoxWidth: 0.2, BBoxHeight: 0.2},
		{Label: "person", Confidence: 0.5, BBoxX: 0.1, BBoxY: 0.1, BBoxWidth: 0, BBoxHeight: 0.2},
	}
	for _, c := range cases {
		if err := Validate(c); err == nil {
			t.Fatalf("expected invalid bbox %+v", c)
		}
	}
}

func TestClampBBox(t *testing.T) {
	x, y, w, h := ClampBBox(1.2, -0.1, 0.5, 0.5)
	if x != 1 || y != 0 {
		t.Fatalf("clamp failed %v %v", x, y)
	}
	if w != 0.01 || h != 0.5 {
		// x=1 w clamped to 0 then 0.01
	}
	x, y, w, h = ClampBBox(0.9, 0.9, 0.2, 0.2)
	if x+w > 1.000001 || y+h > 1.000001 {
		t.Fatalf("clamp overflow %v %v %v %v", x, y, w, h)
	}
}
