package chroma

import (
	"math"
	"testing"
)

func TestConstantChromaIsNotAttenuated(t *testing.T) {
	src := NewPlane(10, 6)
	for i := range src.Pix {
		src.Pix[i] = 192
	}
	for _, opt := range []Options{
		{Convention: Centered, Origin: GlobalOrigin, BlockW: 8, BlockH: 8},
		{Convention: LeftTop, Origin: GlobalOrigin, BlockW: 8, BlockH: 8},
		{Convention: Centered, Origin: BlockLocalOrigin, BlockW: 8, BlockH: 8},
	} {
		got, err := UpsamplePlane(src, 19, 11, opt)
		if err != nil {
			t.Fatal(err)
		}
		for i, v := range got.Pix {
			if math.Abs(v-192) > 1e-12 {
				t.Fatalf("%+v: sample %d changed constant chroma to %g", opt, i, v)
			}
		}
	}
}

func TestGlobalTilingIsPixelIdentical(t *testing.T) {
	fixture, err := EdgeFixture(19, 11)
	if err != nil {
		t.Fatal(err)
	}
	for _, conv := range []Convention{Centered, LeftTop} {
		whole, err := UpsamplePlane(fixture.Cb, 19, 11, Options{
			Convention: conv, Origin: GlobalOrigin, BlockW: 19, BlockH: 11,
		})
		if err != nil {
			t.Fatal(err)
		}
		tiled, err := UpsamplePlane(fixture.Cb, 19, 11, Options{
			Convention: conv, Origin: GlobalOrigin, BlockW: 8, BlockH: 8,
		})
		if err != nil {
			t.Fatal(err)
		}
		for i := range whole.Pix {
			if whole.Pix[i] != tiled.Pix[i] {
				t.Fatalf("%s: pixel %d differs: whole=%g tiled=%g",
					conv, i, whole.Pix[i], tiled.Pix[i])
			}
		}
	}
}

func TestCenteredLinearRampIsExactInTheInterior(t *testing.T) {
	fixture, err := RampFixture(19, 11)
	if err != nil {
		t.Fatal(err)
	}
	fixed, err := UpsamplePlane(fixture.Cb, 19, 11, Options{
		Convention: Centered, Origin: GlobalOrigin, BlockW: 8, BlockH: 8,
	})
	if err != nil {
		t.Fatal(err)
	}
	wrong, err := UpsamplePlane(fixture.Cb, 19, 11, Options{
		Convention: LeftTop, Origin: GlobalOrigin, BlockW: 8, BlockH: 8,
	})
	if err != nil {
		t.Fatal(err)
	}
	for y := 0; y < 11; y++ {
		// x=0 uses the clamped left edge; x>=17 blends the final odd-width
		// chroma sample, which has no complete 2x2 support in the source.
		for x := 1; x < 17; x++ {
			want := fixture.Original.At(x, y).Cb
			if math.Abs(fixed.At(x, y)-want) > 1e-9 {
				t.Fatalf("fixed (%d,%d)=%g, want %g", x, y, fixed.At(x, y), want)
			}
			if math.Abs(wrong.At(x, y)-want) < 0.5 {
				t.Fatalf("wrong (%d,%d)=%g unexpectedly close to ramp value %g",
					x, y, wrong.At(x, y), want)
			}
		}
	}
}

func TestBlockLocalOriginCreatesSeams(t *testing.T) {
	fixture, err := EdgeFixture(19, 11)
	if err != nil {
		t.Fatal(err)
	}
	global, err := UpsamplePlane(fixture.Cb, 19, 11, Options{
		Convention: Centered, Origin: GlobalOrigin, BlockW: 8, BlockH: 8,
	})
	if err != nil {
		t.Fatal(err)
	}
	local, err := UpsamplePlane(fixture.Cb, 19, 11, Options{
		Convention: Centered, Origin: BlockLocalOrigin, BlockW: 8, BlockH: 8,
	})
	if err != nil {
		t.Fatal(err)
	}
	var maxDiff float64
	for i := range global.Pix {
		maxDiff = math.Max(maxDiff, math.Abs(global.Pix[i]-local.Pix[i]))
	}
	if maxDiff < 1 {
		t.Fatalf("block-local origin unexpectedly matched global mapping (max diff %g)", maxDiff)
	}
}

func TestOddDestinationWidthDoesNotShiftPixels(t *testing.T) {
	fixture, err := EdgeFixture(20, 12)
	if err != nil {
		t.Fatal(err)
	}
	odd, err := UpsamplePlane(fixture.Cb, 19, 11, Options{
		Convention: Centered, Origin: GlobalOrigin, BlockW: 8, BlockH: 8,
	})
	if err != nil {
		t.Fatal(err)
	}
	even, err := UpsamplePlane(fixture.Cb, 20, 12, Options{
		Convention: Centered, Origin: GlobalOrigin, BlockW: 8, BlockH: 8,
	})
	if err != nil {
		t.Fatal(err)
	}
	for y := 0; y < 11; y++ {
		for x := 0; x < 19; x++ {
			if odd.At(x, y) != even.At(x, y) {
				t.Fatalf("(%d,%d): odd-width=%g even-width=%g", x, y, odd.At(x, y), even.At(x, y))
			}
		}
	}
}

func TestOddWidthChromaUsesOnlyExistingPixels(t *testing.T) {
	src := &YUVImage{W: 3, H: 1, Pix: []YCbCr{
		{Cb: 10, Cr: 20},
		{Cb: 30, Cr: 40},
		{Cb: 90, Cr: 100},
	}}
	cb, cr, err := Subsample420(src)
	if err != nil {
		t.Fatal(err)
	}
	if cb.W != 2 || cr.W != 2 {
		t.Fatalf("odd width produced chroma size %dx%d, want 2x1", cb.W, cb.H)
	}
	if cb.At(0, 0) != 20 || cb.At(1, 0) != 90 {
		t.Fatalf("cb=%v, want [20 90]", cb.Pix)
	}
	if cr.At(0, 0) != 30 || cr.At(1, 0) != 100 {
		t.Fatalf("cr=%v, want [30 100]", cr.Pix)
	}
}
