package spatial

import (
	"math"
	"testing"
)

func TestObjectAtListenerUsesEqualPowerFallback(t *testing.T) {
	renderer, err := NewRenderer(Standard514(), Config{})
	if err != nil {
		t.Fatal(err)
	}
	mix, err := renderer.Render(Scene{Objects: []Object{{
		ID: "origin", Position: Vec3{}, Gain: 0.5,
	}}})
	if err != nil {
		t.Fatal(err)
	}
	source := mix.Sources[0]
	if len(source.Contributions) != 9 {
		t.Fatalf("omni speaker count = %d, want 9", len(source.Contributions))
	}
	want := 0.5 / math.Sqrt(9)
	for _, contribution := range source.Contributions {
		if contribution.Speaker == ChannelLFE {
			t.Fatal("LFE received directional object energy")
		}
		requireClose(t, "omni gain", contribution.Gain, want)
	}
	requireWarning(t, source.Warnings, WarnNoDirection)
}

func TestObjectOutsideSpeakerCoverageIsFlagged(t *testing.T) {
	renderer, err := NewRenderer(Standard514(), Config{})
	if err != nil {
		t.Fatal(err)
	}
	mix, err := renderer.Render(Scene{Objects: []Object{{
		ID: "below", Position: Vec3{Y: -1}, Gain: 1,
	}}})
	if err != nil {
		t.Fatal(err)
	}
	source := mix.Sources[0]
	requireWarning(t, source.Warnings, WarnOutsideCoverage)
	requireClose(t, "unrepresentable energy", contributionEnergy(source.Contributions), 1)

	renderer, err = NewRenderer(BinauralSimple(), Config{})
	if err != nil {
		t.Fatal(err)
	}
	mix, err = renderer.Render(Scene{Objects: []Object{{
		ID: "behind", Position: Vec3{Z: -1}, Gain: 1,
	}}})
	if err != nil {
		t.Fatal(err)
	}
	requireWarning(t, mix.Sources[0].Warnings, WarnOutsideCoverage)
	requireClose(t, "rear left", contributionMap(mix.Sources[0])["L"], 1/math.Sqrt2)
	requireClose(t, "rear right", contributionMap(mix.Sources[0])["R"], 1/math.Sqrt2)
}

func TestBedAndObjectsAddWhenTheyOverlap(t *testing.T) {
	renderer, err := NewRenderer(Standard514(), Config{})
	if err != nil {
		t.Fatal(err)
	}
	scene := Scene{
		Bed: []BedChannel{
			{ID: "bed-l-1", Channel: ChannelL, Gain: 0.1},
			{ID: "bed-l-2", Channel: ChannelL, Gain: 0.2},
			{ID: "bed-c", Channel: ChannelC, Gain: 0.05},
		},
		Objects: []Object{
			{ID: "object-l", Position: spherical(-30, 0), Gain: 0.4},
			{ID: "object-c", Position: spherical(0, 0), Gain: 0.25},
		},
	}
	mix, err := renderer.Render(scene)
	if err != nil {
		t.Fatal(err)
	}
	want := make(map[string]float64)
	for _, source := range mix.Sources {
		for _, contribution := range source.Contributions {
			want[contribution.Speaker] += contribution.Gain
		}
	}
	got := speakerMap(mix)
	for name, gain := range want {
		requireClose(t, "overlap "+name, got[name], gain)
	}
	for name, gain := range got {
		requireClose(t, "no overwrite "+name, gain, want[name])
	}
	if got[ChannelL] <= 0.7 {
		t.Fatalf("overlapping left gain = %.6f, want bed and object summed", got[ChannelL])
	}
	if mix.Peak != got[peakSpeaker(got)] {
		t.Fatalf("peak = %.6f, want max speaker gain %.6f", mix.Peak, got[peakSpeaker(got)])
	}
}

func TestInvalidInputsReturnErrors(t *testing.T) {
	renderer, err := NewRenderer(Standard514(), Config{})
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name  string
		scene Scene
	}{
		{
			name:  "nan position",
			scene: Scene{Objects: []Object{{ID: "nan", Position: Vec3{X: math.NaN()}, Gain: 1}}},
		},
		{
			name:  "infinite position",
			scene: Scene{Objects: []Object{{ID: "inf", Position: Vec3{Y: math.Inf(1)}, Gain: 1}}},
		},
		{
			name:  "negative gain",
			scene: Scene{Objects: []Object{{ID: "negative", Gain: -1}}},
		},
		{
			name:  "unknown bed channel",
			scene: Scene{Bed: []BedChannel{{Channel: "XYZ", Gain: 1}}},
		},
		{
			name:  "empty bed channel",
			scene: Scene{Bed: []BedChannel{{Gain: 1}}},
		},
		{
			name: "mix overflow",
			scene: Scene{Objects: []Object{
				{ID: "max-1", Position: spherical(-30, 0), Gain: math.MaxFloat64},
				{ID: "max-2", Position: spherical(-30, 0), Gain: math.MaxFloat64},
			}},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := renderer.Render(test.scene); err == nil {
				t.Fatal("expected an error")
			}
		})
	}
}

func TestInvalidRendererConfiguration(t *testing.T) {
	tests := []struct {
		name   string
		layout Layout
		config Config
	}{
		{name: "empty layout", layout: Layout{Name: "empty"}, config: Config{}},
		{name: "negative reference", layout: Standard514(), config: Config{ReferenceDistance: -1}},
		{name: "nan sharpness", layout: Standard514(), config: Config{PanningSharpness: math.NaN()}},
		{name: "bad weight floor", layout: Standard514(), config: Config{WeightFloor: 1}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := NewRenderer(test.layout, test.config); err == nil {
				t.Fatal("expected an error")
			}
		})
	}
}

func peakSpeaker(speakers map[string]float64) string {
	name := ""
	gain := math.Inf(-1)
	for candidate, candidateGain := range speakers {
		if candidateGain > gain {
			name, gain = candidate, candidateGain
		}
	}
	return name
}
