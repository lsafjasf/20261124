package colorxform

import (
	"math"
	"testing"
)

func almost(a, b, tol float64) bool { return math.Abs(a-b) <= tol }

func TestTransferRoundTripIncludingNegatives(t *testing.T) {
	for v := -0.5; v <= 1.5; v += 0.013 {
		if got := SRGBEncode(SRGBDecode(v)); !almost(got, v, 1e-12) {
			t.Fatalf("sRGB round trip failed at %v: got %v", v, got)
		}
		if got := Rec709Encode(Rec709Decode(v)); !almost(got, v, 1e-12) {
			t.Fatalf("Rec.709 round trip failed at %v: got %v", v, got)
		}
	}
}

func TestNegativeValuesAreNotClipped(t *testing.T) {
	// Wide-gamut ACEScg content carries legal negative components.
	r, g, b := ConvertFromACEScg(0.17, 1.12, -0.08, 0)
	if b >= 0 {
		t.Fatalf("negative blue channel was clipped: got %v", b)
	}
	// The encoded negatives must mirror-encode back to the same linear
	// values, i.e. survive the round trip instead of snapping to zero.
	lr, lg, lb := ACEScgToSRGB.Apply(0.17, 1.12, -0.08)
	if !almost(Rec709Decode(r), lr, 1e-12) ||
		!almost(Rec709Decode(g), lg, 1e-12) ||
		!almost(Rec709Decode(b), lb, 1e-12) {
		t.Fatalf("negative-preserving round trip failed: %v %v %v", r, g, b)
	}
}

func TestGamutMatricesRoundTrip(t *testing.T) {
	cases := [][3]float64{
		{1, 1, 1}, {1, 0, 0}, {0, 1, 0}, {0, 0, 1},
		{0.5, 0.25, 0.75}, {0.02, 0.02, 0.02},
	}
	for _, c := range cases {
		ar, ag, ab := SRGBToACEScg.Apply(c[0], c[1], c[2])
		r, g, b := ACEScgToSRGB.Apply(ar, ag, ab)
		if !almost(r, c[0], 1e-12) || !almost(g, c[1], 1e-12) || !almost(b, c[2], 1e-12) {
			t.Fatalf("matrix round trip failed for %v: got %v %v %v", c, r, g, b)
		}
	}
}

func TestWhitePointMapsToWhite(t *testing.T) {
	r, g, b := SRGBToACEScg.Apply(1, 1, 1)
	if !almost(r, 1, 1e-9) || !almost(g, 1, 1e-9) || !almost(b, 1, 1e-9) {
		t.Fatalf("D65 white did not map to ACEScg white: %v %v %v", r, g, b)
	}
}

// TestShadowsNotLifted guards against the double-gamma defect: the
// sRGB EOTF must be applied exactly once on input, so a dark gray must
// come out dark, not lifted toward middle gray.
func TestShadowsNotLifted(t *testing.T) {
	const in = 0.05
	r, g, b := ConvertColor(in, in, in, 0)
	want := Rec709Encode(SRGBDecode(in)) // neutral gray: matrices cancel
	for _, got := range []float64{r, g, b} {
		if !almost(got, want, 1e-12) {
			t.Fatalf("shadow gray %v: got %v, want %v (double gamma would give %v)",
				in, got, want, Rec709Encode(in))
		}
	}
	if got := r; got > 0.1 {
		t.Fatalf("shadow lifted toward gray: input %v -> output %v", in, got)
	}
}

func TestExposureCompensationIsLinear(t *testing.T) {
	// +1 stop must exactly double scene-linear light: verify in the
	// linear domain by decoding the output transfer function.
	r0, _, _ := ConvertColor(0.4, 0.4, 0.4, 0)
	r1, _, _ := ConvertColor(0.4, 0.4, 0.4, 1)
	if got, want := Rec709Decode(r1), 2*Rec709Decode(r0); !almost(got, want, 1e-12) {
		t.Fatalf("+1 stop did not double linear light: got %v, want %v", got, want)
	}
}
