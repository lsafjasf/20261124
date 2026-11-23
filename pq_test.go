package main

import (
	"math"
	"testing"
)

// Roundtrip: linear -> PQ10 -> linear. Above 0.1 nit the 10-bit code
// step stays within ~1.5% of luminance; below that only absolute error
// is meaningful (this is why grading uses 12-bit PQ).
func TestRoundtripAccuracy(t *testing.T) {
	for i := 0; i <= 400; i++ {
		nits := 0.01 * math.Pow(10000.0/0.01, float64(i)/400)
		got := DecodePQ10(EncodePQ10(nits))
		if nits >= 0.1 {
			if rel := math.Abs(got-nits) / nits; rel > 0.015 {
				t.Fatalf("nits=%.5f roundtrip=%.5f rel err %.4f", nits, got, rel)
			}
		} else if abs := math.Abs(got - nits); abs > 0.005 {
			t.Fatalf("nits=%.5f roundtrip=%.5f abs err %.5f", nits, got, abs)
		}
	}
}

// Codes must be monotonic in luminance.
func TestMonotonicCodes(t *testing.T) {
	prev := uint16(0)
	for i := 0; i <= 400; i++ {
		nits := 0.005 * math.Pow(10000.0/0.005, float64(i)/400)
		code := EncodePQ10(nits)
		if code < prev {
			t.Fatalf("non-monotonic at %.5f nit: %d < %d", nits, code, prev)
		}
		prev = code
	}
}

// Mid-gray must not jump when the display peak switches: PQ is absolute,
// so the code and the decoded absolute nits are identical for
// 1000/2000/4000 nit peaks, and mid-gray is never clipped.
func TestMidGrayStableAcrossPeaks(t *testing.T) {
	code := EncodePQ10(MidGrayNits)
	ref := DecodePQ10(code)
	if rel := math.Abs(ref-MidGrayNits) / MidGrayNits; rel > 0.005 {
		t.Fatalf("mid-gray roundtrip drift: %.5f -> %.5f", MidGrayNits, ref)
	}
	for _, peak := range DisplayPeaks {
		got := DecodePQ10(code)
		if got != ref {
			t.Fatalf("peak %.0f: decoded mid-gray %.5f != %.5f", peak, got, ref)
		}
		if _, clipped := DisplayMap(got, peak); clipped {
			t.Fatalf("peak %.0f: mid-gray %.2f nit must not clip", peak, got)
		}
	}
}

// Sanity: known PQ anchor points.
func TestKnownAnchors(t *testing.T) {
	if c := EncodePQ10(0); c != 0 {
		t.Fatalf("0 nit -> code %d, want 0", c)
	}
	if c := EncodePQ10(10000); c != PQCodeMax {
		t.Fatalf("10000 nit -> code %d, want %d", c, PQCodeMax)
	}
	// 1000 nit should land near code 769 (signal ~0.7518).
	if c := EncodePQ10(1000); c < 760 || c > 780 {
		t.Fatalf("1000 nit -> code %d, want ~769", c)
	}
}
