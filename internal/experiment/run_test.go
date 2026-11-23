package experiment

import (
	"math"
	"testing"
)

func TestDefaultExperimentVerifiesAllInvariants(t *testing.T) {
	r, err := Run(DefaultConfig())
	if err != nil {
		t.Fatal(err)
	}
	if err := Verify(r); err != nil {
		t.Fatal(err)
	}
	if r.Config.Width%2 != 1 {
		t.Fatal("default fixture must have odd width")
	}
	if got := r.Fixture.Cb.W; got != (r.Config.Width+1)/2 {
		t.Fatalf("chroma width=%d, want %d", got, (r.Config.Width+1)/2)
	}
	if r.Summary.WrongTiledMismatch != 0 || r.Summary.FixedTiledMismatch != 0 {
		t.Fatalf("global tiling changed pixels: wrong=%g fixed=%g",
			r.Summary.WrongTiledMismatch, r.Summary.FixedTiledMismatch)
	}
	if r.Summary.BlockLocalMismatch < 1 {
		t.Fatalf("block-local control did not expose a seam: %g", r.Summary.BlockLocalMismatch)
	}
}

func TestHalfPixelErrorMovesEdgeToOneSide(t *testing.T) {
	r, err := Run(DefaultConfig())
	if err != nil {
		t.Fatal(err)
	}
	x9 := rowAt(t, r, 9, 0)
	x10 := rowAt(t, r, 10, 0)

	if x9.Delta.R > -40 || x9.Delta.B < 70 {
		t.Fatalf("x=9 should show a blue-shifted wrong result, got dRGB=(%g,%g,%g)",
			x9.Delta.R, x9.Delta.G, x9.Delta.B)
	}
	if math.Abs(x9.FixedRefAbsRGB-x10.FixedRefAbsRGB) > 1e-6 {
		t.Fatalf("fixed edge errors are not symmetric: x9=%g x10=%g",
			x9.FixedRefAbsRGB, x10.FixedRefAbsRGB)
	}
	if x10.WrongRefAbsRGB > 1e-12 {
		t.Fatalf("wrong mapping should put the right sample exactly at x=10, error=%g",
			x10.WrongRefAbsRGB)
	}
	if x9.WrongRefAbsRGB < 1.8*x9.FixedRefAbsRGB {
		t.Fatalf("wrong edge error only %g vs fixed %g", x9.WrongRefAbsRGB, x9.FixedRefAbsRGB)
	}
	if r.Summary.WrongMaxRefAbsRGB < 1.8*r.Summary.FixedMaxRefAbsRGB {
		t.Fatalf("wrong max error=%g is not at least 1.8x fixed max=%g",
			r.Summary.WrongMaxRefAbsRGB, r.Summary.FixedMaxRefAbsRGB)
	}
}

func TestOddWidthAndHeightDoNotShiftLeftCrop(t *testing.T) {
	odd, err := Run(Config{Width: 19, Height: 11, BlockW: 8, BlockH: 8})
	if err != nil {
		t.Fatal(err)
	}
	even, err := Run(Config{Width: 20, Height: 12, BlockW: 8, BlockH: 8})
	if err != nil {
		t.Fatal(err)
	}
	if odd.Summary.OddWidthSampleShift != 0 {
		t.Fatalf("odd width sample shift=%g", odd.Summary.OddWidthSampleShift)
	}
	for x := 0; x < 19; x++ {
		if odd.XRows[x] != even.XRows[x] {
			t.Fatalf("x=%d coordinate map changed with width", x)
		}
		for y := 0; y < 11; y++ {
			a := rowAt(t, odd, x, y)
			b := rowAt(t, even, x, y)
			if a.WrongCb != b.WrongCb || a.FixedCb != b.FixedCb ||
				a.WrongCr != b.WrongCr || a.FixedCr != b.FixedCr {
				t.Fatalf("(%d,%d) changed when odd width was extended", x, y)
			}
		}
	}
}

func TestNoGlobalDesaturationOrLumaChange(t *testing.T) {
	r, err := Run(DefaultConfig())
	if err != nil {
		t.Fatal(err)
	}
	left := rowAt(t, r, 0, 0)
	right := rowAt(t, r, 18, 0)
	if math.Abs(left.FixedCb-left.RefCb) > 1e-12 || math.Abs(left.FixedCr-left.RefCr) > 1e-12 {
		t.Fatalf("fixed version changed flat left chroma: cb %g vs %g, cr %g vs %g",
			left.FixedCb, left.RefCb, left.FixedCr, left.RefCr)
	}
	if math.Abs(right.FixedCb-right.RefCb) > 1e-12 || math.Abs(right.FixedCr-right.RefCr) > 1e-12 {
		t.Fatalf("fixed version changed flat right chroma: cb %g vs %g, cr %g vs %g",
			right.FixedCb, right.RefCb, right.FixedCr, right.RefCr)
	}
	if r.Summary.MaxLumaDelta != 0 {
		t.Fatalf("luma changed by %g", r.Summary.MaxLumaDelta)
	}
	report := NewReport(r)
	if report.Policy.GlobalDesaturation || report.Policy.LumaModified {
		t.Fatalf("policy claims a forbidden global operation: %+v", report.Policy)
	}
}

func rowAt(t *testing.T, r *Result, x, y int) Row {
	t.Helper()
	for _, row := range r.Rows {
		if row.X == x && row.Y == y {
			return row
		}
	}
	t.Fatalf("row (%d,%d) not found", x, y)
	return Row{}
}
