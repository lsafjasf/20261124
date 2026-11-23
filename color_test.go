package main

import (
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func approxEq(t *testing.T, got, want, tol float64, label string) {
	t.Helper()
	if math.Abs(got-want) > tol {
		t.Fatalf("%s = %.8f, want %.8f (tol %.1e)", label, got, want, tol)
	}
}

func TestWhitePointAndLuma(t *testing.T) {
	white := RGB{1, 1, 1}
	x2020 := white.toXYZ(rgb2xyz2020)
	// Exact ITU primaries differ from the rounded D65 publication values at
	// the 1e-4 level; both matrices share the same internally consistent white.
	approxEq(t, x2020.X, whiteX, 3e-4, "2020 white X")
	approxEq(t, x2020.Y, 1, 1e-12, "2020 white Y")
	approxEq(t, x2020.Z, whiteZ, 3e-4, "2020 white Z")

	x709 := white.toXYZ(rgb2xyz709)
	approxEq(t, x709.X, whiteX, 3e-4, "709 white X")
	approxEq(t, x709.Z, whiteZ, 3e-4, "709 white Z")

	round := Rec2020To709Linear(white)
	approxEq(t, round.R, 1, 1e-9, "white R")
	approxEq(t, round.G, 1, 1e-9, "white G")
	approxEq(t, round.B, 1, 1e-9, "white B")
}

func TestGammaRoundTrip(t *testing.T) {
	for _, v := range []float64{0.001, 0.05, 0.18, 0.5, 1.0} {
		approxEq(t, Decode709(Encode709(v)), v, 1e-12, "gamma roundtrip")
	}
}

func TestLiftNegativesPreservesLuma(t *testing.T) {
	c := RGB{0.10, -0.12, 1.20}
	out, rep := liftNegatives(c, luma2020)
	if !rep.HadNegative {
		t.Fatal("expected negative flag")
	}
	if out.R < 0 || out.G < 0 || out.B < 0 {
		t.Fatalf("negative channel remains: %+v", out)
	}
	approxEq(t, out.luma(luma2020), c.luma(luma2020), 1e-12, "luma preserved")

	inGamut := RGB{0.2, 0.4, 0.6}
	same, rep2 := liftNegatives(inGamut, luma2020)
	if rep2.HadNegative || same != inGamut {
		t.Fatal("in-gamut pixel must be untouched")
	}
}

func TestBoundaryPreservesLumaHueAndFitsCube(t *testing.T) {
	cases := []RGB{
		{0.55, 0.10, 1.35},
		{-0.15, 1.15, 1.30},
		{1.40, 0.05, 0.55},
		{0.10, -0.12, 1.20},
	}
	for _, src := range cases {
		lifted, _ := liftNegatives(src, luma2020)
		raw := Rec2020To709Linear(lifted)
		lifted709, _ := liftNegatives(raw, luma709)
		out, rep := compressToBoundary(lifted709)

		for k := 0; k < 3; k++ {
			v := out.channel(k)
			if v < -1e-9 || v > 1+1e-9 {
				t.Fatalf("output outside cube %+v from %+v", out, src)
			}
		}
		wantY := math.Min(1, lifted709.luma(luma709))
		approxEq(t, out.luma(luma709), wantY, 1e-9, "boundary luma")

		gray := lifted709.luma(luma709)
		if gray <= 1 {
			srcAng := math.Atan2(lifted709.B-gray, lifted709.R-gray)
			outAng := math.Atan2(out.B-gray, out.R-gray)
			approxEq(t, math.Abs(srcAng-outAng), 0, 1e-9, "constant-luminance hue angle")
		}
		if rep.Alpha > 1e-6 && rep.HitFace == "interior" && !inside(lifted709) {
			t.Fatalf("out-of-gamut input must stop on a cube face: %+v %+v", src, rep)
		}
	}
}

func inside(c RGB) bool {
	return c.R >= 0 && c.R <= 1 && c.G >= 0 && c.G <= 1 && c.B >= 0 && c.B <= 1
}

func TestDeltaEIdentityAndKnownPair(t *testing.T) {
	lab := Lab{50, 20, -30}
	approxEq(t, DeltaE2000(lab, lab), 0, 1e-12, "identical deltaE")

	// Sharma CIEDE2000 worked example pair 1.
	p := Lab{50, 2.6772, -79.7751}
	q := Lab{50, 0, -82.7485}
	approxEq(t, DeltaE2000(p, q), 2.0425, 0.01, "Sharma pair 1")
}

func TestHueFlipDetection(t *testing.T) {
	src := Lab{40, 40, 0}
	flipped := Lab{40, -40, 1}
	if hueFlip(0, src, flipped) == nil {
		t.Fatal("expected hue flip")
	}
	near := Lab{40, 38, 2}
	if hueFlip(1, src, near) != nil {
		t.Fatal("small rotation must not count as flip")
	}
	neutral := Lab{50, 0.05, 0.02}
	if hueFlip(2, src, neutral) != nil {
		t.Fatal("near-neutral output must not count as flip")
	}
}

func TestPPMRoundTripHeader(t *testing.T) {
	dir := t.TempDir()
	im := newImage(2, 1)
	im.Pixels[0] = RGB{1, 0, 0}
	im.Pixels[1] = RGB{0, 1, 0}
	path, err := WritePPM16(dir, "x.p6.ppm", im)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(data[:16]), "P6\n") || !strings.Contains(string(data[:128]), "65535") {
		t.Fatalf("bad P6 header: %q", string(data[:64]))
	}
	if len(data) != len("P6\n# gamutcompress evidence, 16-bit big-endian Rec.709 display values\n2 1\n65535\n")+12 {
		t.Fatalf("unexpected P6 byte length %d", len(data))
	}
}
