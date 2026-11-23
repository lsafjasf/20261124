package fixtures

import (
	"testing"
)

func TestAllScenesCoverNegativeAndHighlightRanges(t *testing.T) {
	for _, scene := range All(96, 64) {
		if scene.Frame.Width != 96 || scene.Frame.Height != 64 {
			t.Fatalf("%s: dimensions = %dx%d", scene.Name, scene.Frame.Width, scene.Frame.Height)
		}
		var negative, over, finite int
		for _, c := range scene.Frame.Pixels {
			if c.IsFinite() {
				finite++
			}
			if c.Min() < 0 {
				negative++
			}
			if c.Max() > 1 {
				over++
			}
		}
		if finite != len(scene.Frame.Pixels) {
			t.Fatalf("%s: %d non-finite pixels", scene.Name, len(scene.Frame.Pixels)-finite)
		}
		if negative == 0 {
			t.Fatalf("%s: no negative components", scene.Name)
		}
		if over == 0 {
			t.Fatalf("%s: no over-range components", scene.Name)
		}
	}
}

func TestThreeIncreasingIntensityLevels(t *testing.T) {
	if len(Levels) != 3 {
		t.Fatalf("levels = %d, want 3", len(Levels))
	}
	for i := 1; i < len(Levels); i++ {
		if Levels[i].Gain <= Levels[i-1].Gain {
			t.Fatalf("levels are not increasing: %+v", Levels)
		}
	}
}

func TestColorCheckerTilesAreSolid(t *testing.T) {
	scene := ColorChecker(120, 120)
	const tile = 20
	for ty := 0; ty < 6; ty++ {
		for tx := 0; tx < 6; tx++ {
			want := scene.Frame.At(tx*tile+tile/2, ty*tile+tile/2)
			for y := ty * tile; y < (ty+1)*tile; y++ {
				for x := tx * tile; x < (tx+1)*tile; x++ {
					if got := scene.Frame.At(x, y); got != want {
						t.Fatalf("tile (%d,%d) is not solid at (%d,%d): %v != %v", tx, ty, x, y, got, want)
					}
				}
			}
		}
	}
}

func TestEmittersHaveNegativeRimAndBrightCore(t *testing.T) {
	scene := Emitters(192, 96)
	var negative, bright int
	for _, c := range scene.Frame.Pixels {
		if c.R < 0 {
			negative++
		}
		if c.Max() > 8 {
			bright++
		}
	}
	if negative == 0 {
		t.Fatal("emitter rim has no negative red components")
	}
	if bright == 0 {
		t.Fatal("emitter core has no high-intensity pixels")
	}
}

func TestGradientContainsBothSidesOfGamut(t *testing.T) {
	scene := Gradient(64, 32)
	left := scene.Frame.At(0, 0)
	right := scene.Frame.At(63, 31)
	if left.R >= 0 {
		t.Fatalf("gradient left edge should include negative red, got %v", left)
	}
	if right.Max() <= 1 {
		t.Fatalf("gradient right edge should exceed 1, got %v", right)
	}
	if !left.IsFinite() || !right.IsFinite() {
		t.Fatalf("gradient contains non-finite values")
	}
}
