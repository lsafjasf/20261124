package main

import "testing"

func TestStrategyComparison(t *testing.T) {
	_, source := Synthesize()
	oog := outOfGamutMask(source)

	stats := map[StrategyName]*StrategyStats{}
	for _, name := range allStrategies {
		s, _ := evaluate(name, source, oog)
		stats[name] = s
	}

	boundary := stats[StrategyBoundary]
	clip := stats[StrategyClip]
	matrix := stats[StrategyMatrix]

	if boundary.HueFlipPixels != 0 {
		t.Fatalf("boundary search must not flip hues, got %d", boundary.HueFlipPixels)
	}
	if clip.HueFlipPixels == 0 {
		t.Fatal("clip strategy must expose hue flips on the extreme swatches")
	}
	if clip.HueFlipPixels != matrix.HueFlipPixels {
		t.Fatalf("clip and display-clamped matrix should agree on flips: %d vs %d",
			clip.HueFlipPixels, matrix.HueFlipPixels)
	}
	if matrix.ExcursionPixels == 0 {
		t.Fatal("fixed matrix must record out-of-gamut excursions before display clamp")
	}
	if boundary.ExcursionPixels != 0 {
		t.Fatalf("boundary search must stay in gamut, got %d excursions", boundary.ExcursionPixels)
	}
	if boundary.MeanLumaAbsError >= clip.MeanLumaAbsError {
		t.Fatalf("boundary mean luma error %.4f should beat clip %.4f",
			boundary.MeanLumaAbsError, clip.MeanLumaAbsError)
	}

	// Every boundary output must fit the Rec.709 display cube.
	for i, src := range source {
		out := RunStrategy(StrategyBoundary, src).OutputLinear709
		for k := 0; k < 3; k++ {
			v := out.channel(k)
			if v < -1e-9 || v > 1+1e-9 {
				t.Fatalf("pixel %d channel %d outside cube: %.8f", i, k, v)
			}
		}
	}
}

func TestDeterministicSynthesis(t *testing.T) {
	a, pixelsA := Synthesize()
	b, pixelsB := Synthesize()
	if len(pixelsA) != len(pixelsB) || len(pixelsA) != a.Width*a.Height {
		t.Fatal("synthesis size mismatch")
	}
	for i := range pixelsA {
		if pixelsA[i] != pixelsB[i] {
			t.Fatalf("synthesis not deterministic at pixel %d", i)
		}
	}
	_ = b
}
