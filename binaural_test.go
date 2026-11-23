package spatial

import (
	"math"
	"testing"
)

func TestBinauralVerticalOnlySourceFoldsToFront(t *testing.T) {
	renderer, err := NewRenderer(BinauralSimple(), Config{})
	if err != nil {
		t.Fatal(err)
	}
	mix, err := renderer.Render(Scene{Objects: []Object{{
		ID: "overhead", Position: Vec3{Y: 4}, Gain: 1,
	}}})
	if err != nil {
		t.Fatal(err)
	}
	source := mix.Sources[0]
	requireVecClose(t, "folded direction", source.Direction, Vec3{Z: 1})
	requireClose(t, "height loss", source.HeightLossDegrees, 90)
	requireClose(t, "left gain", contributionMap(source)["L"], 0.25/math.Sqrt2)
	requireClose(t, "right gain", contributionMap(source)["R"], 0.25/math.Sqrt2)
	requireWarning(t, source.Warnings, WarnHeightOnly)
}

func TestBinauralElevationFoldPreservesAzimuth(t *testing.T) {
	renderer, err := NewRenderer(BinauralSimple(), Config{})
	if err != nil {
		t.Fatal(err)
	}
	mix, err := renderer.Render(Scene{Objects: []Object{{
		ID: "upper-left", Position: Vec3{X: -1, Y: 1, Z: 1}, Gain: 1,
	}}})
	if err != nil {
		t.Fatal(err)
	}
	source := mix.Sources[0]
	wantDirection := Vec3{X: -1 / math.Sqrt2, Z: 1 / math.Sqrt2}
	requireVecClose(t, "folded direction", source.Direction, wantDirection)
	requireClose(t, "height loss", source.HeightLossDegrees, math.Atan(1/math.Sqrt2)*180/math.Pi)
	requireClose(t, "total gain", contributionEnergy(source.Contributions), source.DistanceGain)

	left := channelEnergy(source, "L")
	right := channelEnergy(source, "R")
	if left <= right {
		t.Fatalf("left gain %.6f must exceed right gain %.6f", left, right)
	}
	requireWarning(t, source.Warnings, WarnHeightFolded)
}

func TestBinauralBedFold(t *testing.T) {
	renderer, err := NewRenderer(BinauralSimple(), Config{})
	if err != nil {
		t.Fatal(err)
	}
	mix, err := renderer.Render(Scene{Bed: []BedChannel{
		{ID: "center", Channel: ChannelC, Gain: 0.5},
		{ID: "lfe", Channel: ChannelLFE, Gain: 0.25},
		{ID: "top-left", Channel: ChannelTFL, Gain: 0.5},
	}})
	if err != nil {
		t.Fatal(err)
	}

	center := mix.Sources[0]
	requireClose(t, "center left", contributionMap(center)["L"], 0.5/math.Sqrt2)
	requireClose(t, "center right", contributionMap(center)["R"], 0.5/math.Sqrt2)

	lfe := mix.Sources[1]
	requireClose(t, "LFE left", contributionMap(lfe)["L"], 0.25/math.Sqrt2)
	requireClose(t, "LFE right", contributionMap(lfe)["R"], 0.25/math.Sqrt2)
	requireWarning(t, lfe.Warnings, WarnLFEOmni)

	topLeft := mix.Sources[2]
	requireClose(t, "TFL height loss", topLeft.HeightLossDegrees, 45)
	requireClose(t, "TFL left", contributionMap(topLeft)["L"], 0.5)
	requireClose(t, "TFL right", contributionMap(topLeft)["R"], 0)
	requireWarning(t, topLeft.Warnings, WarnHeightFolded)

	speakers := speakerMap(mix)
	requireClose(t, "mixed left", speakers["L"], 0.5/math.Sqrt2+0.25/math.Sqrt2+0.5)
	requireClose(t, "mixed right", speakers["R"], 0.5/math.Sqrt2+0.25/math.Sqrt2)
}

func TestBedDirectTo514(t *testing.T) {
	bed := make([]BedChannel, 0, 10)
	for _, speaker := range Standard514().Speakers {
		bed = append(bed, BedChannel{Channel: speaker.Name, Gain: 1})
	}
	renderer, err := NewRenderer(Standard514(), Config{})
	if err != nil {
		t.Fatal(err)
	}
	mix, err := renderer.Render(Scene{Bed: bed})
	if err != nil {
		t.Fatal(err)
	}
	for _, speaker := range mix.Speakers {
		requireClose(t, "bed "+speaker.Speaker, speaker.Gain, 1)
	}
	requireClose(t, "peak", mix.Peak, 1)
}

func requireWarning(t *testing.T, warnings []Warning, code WarningCode) {
	t.Helper()
	for _, warning := range warnings {
		if warning.Code == code {
			return
		}
	}
	t.Fatalf("missing warning %q in %+v", code, warnings)
}

func speakerMap(mix Mix) map[string]float64 {
	result := make(map[string]float64, len(mix.Speakers))
	for _, speaker := range mix.Speakers {
		result[speaker.Speaker] = speaker.Gain
	}
	return result
}
