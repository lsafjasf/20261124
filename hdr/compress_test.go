package hdr

import (
	"math"
	"testing"
)

func TestCompressLeavesValuesBelowKneeUntouched(t *testing.T) {
	in := RGB{R: 0.20, G: 0.40, B: 0.60}
	res := Compress(in, Balanced)
	if res.Color != in {
		t.Fatalf("Compress(%v) = %v, want identity", in, res.Color)
	}
	if res.NegativeRecovered || res.HighlightCompressed || res.Unrecoverable != "" {
		t.Fatalf("unexpected status: %+v", res)
	}
}

func TestNegativeRecoveryDoesNotClampPerChannel(t *testing.T) {
	in := RGB{R: -0.10, G: 0.00, B: 1.50}
	res := Compress(in, Balanced)
	naive := NaiveClamp(in)

	if !res.NegativeRecovered {
		t.Fatal("negative component was not reported as recovered wide-gamut color")
	}
	if res.Color.G <= naive.G {
		t.Fatalf("green channel = %v, want > naive clamp %v", res.Color.G, naive.G)
	}
	if res.Color.B >= naive.B {
		t.Fatalf("blue channel = %v, want < naive clamp %v", res.Color.B, naive.B)
	}
	assertHueClose(t, in, res.Color, 1e-9)
	assertDisplayable(t, res.Color)
}

func TestNegativeRecoveryPreservesLuminanceAndHue(t *testing.T) {
	in := RGB{R: -0.20, G: 0.05, B: 0.80}
	p := Preset{Name: "test", Knee: 0.999, DetailLimit: 1e9}
	res := Compress(in, p)

	if !res.NegativeRecovered {
		t.Fatal("negative component was not recovered")
	}
	if got, want := res.Color.Luminance(), in.Luminance(); math.Abs(got-want) > 1e-12 {
		t.Fatalf("luminance = %.15f, want %.15f", got, want)
	}
	assertHueClose(t, in, res.Color, 1e-9)
	assertDisplayable(t, res.Color)
}

func TestNegativeAndOverflowAreSeparateEvents(t *testing.T) {
	both := Compress(RGB{R: -0.10, G: 0.00, B: 2.00}, Balanced)
	if !both.NegativeRecovered || !both.HighlightCompressed {
		t.Fatalf("both stages should fire: %+v", both)
	}
	negativeOnly := Compress(RGB{R: -0.10, G: 0.00, B: 0.50}, Balanced)
	if !negativeOnly.NegativeRecovered || negativeOnly.HighlightCompressed {
		t.Fatalf("negative-only color misclassified: %+v", negativeOnly)
	}
	overflowOnly := Compress(RGB{R: 0.10, G: 0.20, B: 2.00}, Balanced)
	if overflowOnly.NegativeRecovered || !overflowOnly.HighlightCompressed {
		t.Fatalf("overflow-only color misclassified: %+v", overflowOnly)
	}
}

func TestNonFiniteIsUnrecoverable(t *testing.T) {
	for _, in := range []RGB{
		{R: math.NaN(), G: 0, B: 0},
		{R: 0, G: math.Inf(1), B: 0},
		{R: 0, G: 0, B: math.Inf(-1)},
	} {
		res := Compress(in, Balanced)
		if res.Unrecoverable != ReasonNonFinite {
			t.Fatalf("Compress(%v) reason = %q, want %q", in, res.Unrecoverable, ReasonNonFinite)
		}
		assertDisplayable(t, res.Color)
	}
}

func TestNegativeLuminanceIsUnrecoverable(t *testing.T) {
	in := RGB{R: -0.5, G: -0.1, B: -0.2}
	res := Compress(in, Balanced)
	if res.Unrecoverable != ReasonNegativeLuminance {
		t.Fatalf("reason = %q, want %q", res.Unrecoverable, ReasonNegativeLuminance)
	}
	assertDisplayable(t, res.Color)
}

func TestHighlightDetailLimitIsPresetSpecific(t *testing.T) {
	in := RGB{R: 0, G: 0, B: 50}
	if res := Compress(in, Mild); res.Unrecoverable != ReasonHighlightDetail {
		t.Fatalf("mild reason = %q, want %q", res.Unrecoverable, ReasonHighlightDetail)
	}
	if res := Compress(in, Balanced); res.Unrecoverable != ReasonHighlightDetail {
		t.Fatalf("balanced reason = %q, want %q", res.Unrecoverable, ReasonHighlightDetail)
	}
	if res := Compress(in, Strong); res.Unrecoverable != "" {
		t.Fatalf("strong reason = %q, want none", res.Unrecoverable)
	}
}

func TestExtremeValuesRemainDisplayable(t *testing.T) {
	cases := []RGB{
		{R: 1e12, G: 1e12, B: 1e12},
		{R: math.MaxFloat64, G: math.MaxFloat64, B: math.MaxFloat64},
		{R: -1e6, G: 1e6, B: 1e6},
		{R: -1, G: 2, B: 3},
		{R: 0, G: 0, B: 0},
		{R: 0.001, G: 0.002, B: 0.003},
	}
	for _, p := range Presets() {
		for _, in := range cases {
			res := Compress(in, p)
			assertDisplayable(t, res.Color)
			if !res.Color.IsFinite() {
				t.Fatalf("%s: non-finite output for %v", p.Name, in)
			}
		}
	}
}

func TestShoulderIsContinuousAndMonotonic(t *testing.T) {
	for _, p := range Presets() {
		if got := shoulder(p.Knee, p.Knee); got != p.Knee {
			t.Fatalf("%s: shoulder at knee = %v, want %v", p.Name, got, p.Knee)
		}
		prev := shoulder(p.Knee, p.Knee)
		for m := p.Knee + 0.000001; m < 1e9; m *= 1.7 {
			got := shoulder(m, p.Knee)
			if got < prev {
				t.Fatalf("%s: shoulder not monotonic at m=%v (%v < %v)", p.Name, m, got, prev)
			}
			if got >= 1 {
				t.Fatalf("%s: shoulder(%v) = %v, want < 1", p.Name, m, got)
			}
			prev = got
		}
		atKnee := shoulder(p.Knee, p.Knee)
		aboveKnee := shoulder(p.Knee+1e-9, p.Knee)
		if math.Abs(aboveKnee-atKnee) > 1e-7 {
			t.Fatalf("%s: shoulder jumps at knee by %v", p.Name, aboveKnee-atKnee)
		}
	}
}

func assertDisplayable(t *testing.T, c RGB) {
	t.Helper()
	if !c.InGamut() {
		t.Fatalf("color %v is outside [0,1]^3", c)
	}
}

func assertHueClose(t *testing.T, a, b RGB, tolerance float64) {
	t.Helper()
	ha, ca, oka := testHue(a)
	hb, cb, okb := testHue(b)
	if !oka || !okb {
		t.Fatalf("colors %v and %v do not both have defined hue", a, b)
	}
	if ca < 1e-9 || cb < 1e-9 {
		t.Fatalf("colors %v and %v are too close to neutral", a, b)
	}
	d := math.Abs(ha - hb)
	if d > 180 {
		d = 360 - d
	}
	if d > tolerance {
		t.Fatalf("hue shift = %.12f degrees, want <= %.12f (%v -> %v)", d, tolerance, a, b)
	}
}

func testHue(c RGB) (float64, float64, bool) {
	maximum, minimum := c.Max(), c.Min()
	chroma := maximum - minimum
	if !c.IsFinite() || chroma <= 0 {
		return 0, 0, false
	}
	var h float64
	switch maximum {
	case c.R:
		h = 60 * math.Mod((c.G-c.B)/chroma, 6)
	case c.G:
		h = 60 * ((c.B-c.R)/chroma + 2)
	default:
		h = 60 * ((c.R-c.G)/chroma + 4)
	}
	if h < 0 {
		h += 360
	}
	return h, chroma, true
}
