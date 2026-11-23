package chromasiting

import (
	"math"
	"testing"
)

func allSamples() []Sample {
	return []Sample{
		RedBlueBoundary(16, 8),
		OddSize(13, 7),
		HalfPixelOffset(16, 8),
	}
}

func allSitings() []Siting {
	return []Siting{SitingMPEG2, SitingJPEG, SitingCenter}
}

func TestOddSizeChromaDimensions(t *testing.T) {
	sub := Subsample420(OddSize(13, 7).Img, SitingMPEG2)
	if sub.Cb.W != 7 || sub.Cb.H != 4 {
		t.Fatalf("chroma = %dx%d, want 7x4", sub.Cb.W, sub.Cb.H)
	}
}

// Upsampling with the same siting must not move the measured edge.
func TestUpsamplingPreservesBoundary(t *testing.T) {
	for _, smp := range allSamples() {
		for _, s := range allSitings() {
			r := MeasureBoundary(smp, s)
			if math.Abs(r.After-r.Before) > 1e-9 {
				t.Errorf("%s %v: before=%.3f after=%.3f",
					smp.Name, s, r.Before, r.After)
			}
		}
	}
}

// A wrong siting assumption must shift the edge by roughly half a luma
// pixel (the horizontal offset difference between left and center).
func TestSitingMismatchShiftsBoundary(t *testing.T) {
	for _, smp := range allSamples() {
		consistent := MeasureBoundary(smp, SitingMPEG2).After
		shifted := MeasureMismatch(smp, SitingMPEG2, SitingJPEG)
		if d := math.Abs(shifted - consistent); math.Abs(d-0.5) > 1e-9 {
			t.Errorf("%s: MPEG-2->JPEG shift = %.3f, want 0.5", smp.Name, d)
		}
	}
}

// JPEG and Center are physically identical, so they must produce identical
// measurements everywhere.
func TestJPEGAndCenterEquivalent(t *testing.T) {
	for _, smp := range allSamples() {
		a := MeasureBoundary(smp, SitingJPEG)
		b := MeasureBoundary(smp, SitingCenter)
		if a.Before != b.Before || a.After != b.After {
			t.Errorf("%s: JPEG (%v,%v) != Center (%v,%v)",
				smp.Name, a.Before, a.After, b.Before, b.After)
		}
	}
}

// The measured edge must stay within one luma pixel of the true edge for
// every sample and consistent siting.
func TestBoundaryStaysNearTruth(t *testing.T) {
	for _, smp := range allSamples() {
		for _, s := range allSitings() {
			r := MeasureBoundary(smp, s)
			if math.Abs(r.DeltaTrue) > 1.0 {
				t.Errorf("%s %v: delta=%.3f exceeds 1px", smp.Name, s, r.DeltaTrue)
			}
		}
	}
}
