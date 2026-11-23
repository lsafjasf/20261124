package chroma

import (
	"math"
	"testing"
)

func TestCenteredWeights(t *testing.T) {
	tests := []struct {
		x  int
		q  float64
		i0 int
		i1 int
		w0 float64
		w1 float64
	}{
		{0, -0.25, 0, 0, 1, 0},
		{1, 0.25, 0, 1, 0.75, 0.25},
		{2, 0.75, 0, 1, 0.25, 0.75},
		{3, 1.25, 1, 2, 0.75, 0.25},
		{18, 8.75, 8, 9, 0.25, 0.75},
	}
	for _, tt := range tests {
		got := Axis(tt.x, 10, Centered)
		if !near(got.Coordinate, tt.q) || got.I0 != tt.i0 || got.I1 != tt.i1 ||
			!near(got.W0, tt.w0) || !near(got.W1, tt.w1) {
			t.Errorf("x=%d: got q=%g taps=[%d:%g %d:%g], want q=%g taps=[%d:%g %d:%g]",
				tt.x, got.Coordinate, got.I0, got.W0, got.I1, got.W1,
				tt.q, tt.i0, tt.w0, tt.i1, tt.w1)
		}
	}
}

func TestWrongConventionIsHalfLumaPixelRight(t *testing.T) {
	for x := 0; x < 19; x++ {
		wrong := Axis(x, 10, LeftTop)
		fixed := Axis(x, 10, Centered)
		if !near(wrong.Coordinate-fixed.Coordinate, 0.25) {
			t.Fatalf("x=%d: chroma coordinate offset=%g, want 0.25", x, wrong.Coordinate-fixed.Coordinate)
		}
		wrongLuma := 2*wrong.Coordinate + 1
		fixedLuma := 2*fixed.Coordinate + 1
		if !near(wrongLuma-fixedLuma, 0.5) {
			t.Fatalf("x=%d: luma position offset=%g, want 0.5", x, wrongLuma-fixedLuma)
		}
	}
}

func TestOddWidthLastPixelKeepsItsCenter(t *testing.T) {
	const width = 19
	m := Axis(width-1, (width+1)/2, Centered)
	got := 2*m.Coordinate + 1
	want := float64(width-1) + 0.5
	if !near(got, want) {
		t.Fatalf("last pixel samples luma position %g, want its center %g", got, want)
	}
}

func TestWidthDoesNotChangeAxisMapping(t *testing.T) {
	for width := 1; width <= 33; width += 2 {
		chromaWidth := (width + 1) / 2
		evenChromaWidth := (width + 2) / 2
		if chromaWidth != evenChromaWidth {
			t.Fatalf("width=%d: odd/even chroma widths differ: %d != %d",
				width, chromaWidth, evenChromaWidth)
		}
		for x := 0; x < width; x++ {
			odd := Axis(x, chromaWidth, Centered)
			even := Axis(x, evenChromaWidth, Centered)
			if odd != even {
				t.Fatalf("width=%d x=%d: odd mapping %+v differs from even mapping %+v",
					width, x, odd, even)
			}
		}
	}
}

func near(a, b float64) bool {
	return math.Abs(a-b) < 1e-12
}
