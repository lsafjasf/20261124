package spatial

import (
	"math"
	"testing"
)

const testTolerance = 1e-9

func TestStandard514Layout(t *testing.T) {
	layout := Standard514()
	if err := layout.Validate(); err != nil {
		t.Fatalf("layout validation failed: %v", err)
	}
	if !layout.HasHeight {
		t.Fatal("5.1.4 must have height speakers")
	}
	if len(layout.Speakers) != 10 {
		t.Fatalf("speaker count = %d, want 10", len(layout.Speakers))
	}
	names := make(map[string]bool)
	for _, speaker := range layout.Speakers {
		names[speaker.Name] = true
		if speaker.Name != layout.LFE {
			if math.Abs(speaker.Position.Length()-1) > testTolerance {
				t.Errorf("speaker %s position is not unit length", speaker.Name)
			}
		}
	}
	for _, name := range []string{"L", "R", "C", "LFE", "Ls", "Rs", "TFL", "TFR", "TBL", "TBR"} {
		if !names[name] {
			t.Errorf("missing speaker %s", name)
		}
	}
}

func TestDistanceAttenuation(t *testing.T) {
	renderer, err := NewRenderer(Standard514(), Config{})
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		distance float64
		want     float64
	}{
		{0, 1},
		{0.25, 1},
		{1, 1},
		{2, 0.5},
		{8, 0.125},
	}
	for _, test := range tests {
		mix, err := renderer.Render(Scene{Objects: []Object{{
			ID:       "distance",
			Position: Vec3{Z: test.distance},
			Gain:     0.75,
		}}})
		if err != nil {
			t.Fatal(err)
		}
		source := mix.Sources[0]
		requireClose(t, "distance gain", source.DistanceGain, test.want)
		requireClose(t, "energy", contributionEnergy(source.Contributions), 0.75*test.want)
	}
}

func TestAsymmetricObjectPreservesDirectionAndEnergy(t *testing.T) {
	position := Vec3{X: -2.3, Y: 0.8, Z: 1.4}
	renderer, err := NewRenderer(Standard514(), Config{})
	if err != nil {
		t.Fatal(err)
	}
	mix, err := renderer.Render(Scene{Objects: []Object{{
		ID: "asymmetric", Position: position, Gain: 0.75,
	}}})
	if err != nil {
		t.Fatal(err)
	}

	source := mix.Sources[0]
	distance := position.Length()
	requireClose(t, "distance", source.Distance, distance)
	requireClose(t, "distance gain", source.DistanceGain, 1/distance)
	requireClose(t, "source energy", contributionEnergy(source.Contributions), 0.75/distance)
	requireClose(t, "height loss", source.HeightLossDegrees, 0)

	wantDirection, _ := position.Unit()
	requireVecClose(t, "effective direction", source.Direction, wantDirection)
	if azimuthDegrees(source.Direction) >= 0 {
		t.Fatalf("source azimuth = %.3f, want negative", azimuthDegrees(source.Direction))
	}

	left := channelEnergy(source, "L", "Ls", "TFL", "TBL")
	right := channelEnergy(source, "R", "Rs", "TFR", "TBR")
	if left <= right {
		t.Fatalf("left energy %.6f must exceed right energy %.6f", left, right)
	}
	if channelEnergy(source, "TFL", "TFR", "TBL", "TBR") == 0 {
		t.Fatal("positive elevation produced no height-speaker energy")
	}
}

func TestOutOfBoundsKeepsBearingAndOnlyChangesDistance(t *testing.T) {
	renderer, err := NewRenderer(Standard514(), Config{})
	if err != nil {
		t.Fatal(err)
	}
	near := Vec3{X: 1}
	far := Vec3{X: 100}
	mix, err := renderer.Render(Scene{Objects: []Object{
		{ID: "near", Position: near, Gain: 1},
		{ID: "far", Position: far, Gain: 1},
	}})
	if err != nil {
		t.Fatal(err)
	}

	nearSource, farSource := mix.Sources[0], mix.Sources[1]
	requireClose(t, "near gain", nearSource.DistanceGain, 1)
	requireClose(t, "far gain", farSource.DistanceGain, 0.01)
	requireClose(t, "height loss", farSource.HeightLossDegrees, 0)
	if len(nearSource.Contributions) != len(farSource.Contributions) {
		t.Fatal("normalized bearing changed the active speaker set")
	}
	for i := range nearSource.Contributions {
		if nearSource.Contributions[i].Speaker != farSource.Contributions[i].Speaker {
			t.Fatal("normalized bearing changed active speakers")
		}
		nearDirectional := nearSource.Contributions[i].Gain / nearSource.DistanceGain
		farDirectional := farSource.Contributions[i].Gain / farSource.DistanceGain
		requireClose(t, "same bearing gain", nearDirectional, farDirectional)
	}

	left := channelEnergy(nearSource, "L", "Ls", "TFL", "TBL")
	right := channelEnergy(nearSource, "R", "Rs", "TFR", "TBR")
	if left > 1e-8 {
		t.Fatalf("out-of-bounds +X source leaked %.3e into left speakers", left)
	}
	if right < 0.999999 {
		t.Fatalf("out-of-bounds +X source right energy = %.6f, want near unity", right)
	}
}

func TestMirroredPositionsProduceMirroredGains(t *testing.T) {
	renderer, err := NewRenderer(Standard514(), Config{})
	if err != nil {
		t.Fatal(err)
	}
	mix, err := renderer.Render(Scene{Objects: []Object{
		{ID: "left", Position: Vec3{X: -1.3, Y: 0.7, Z: 2.1}, Gain: 1},
		{ID: "right", Position: Vec3{X: 1.3, Y: 0.7, Z: 2.1}, Gain: 1},
	}})
	if err != nil {
		t.Fatal(err)
	}
	leftGains := contributionMap(mix.Sources[0])
	rightGains := contributionMap(mix.Sources[1])
	mirror := map[string]string{
		"L": "R", "R": "L", "C": "C",
		"Ls": "Rs", "Rs": "Ls",
		"TFL": "TFR", "TFR": "TFL",
		"TBL": "TBR", "TBR": "TBL",
	}
	for sourceName, sourceGain := range leftGains {
		target, ok := mirror[sourceName]
		if !ok {
			t.Fatalf("no mirror for speaker %s", sourceName)
		}
		requireClose(t, "mirrored gain "+sourceName, rightGains[target], sourceGain)
	}
}

func TestOverlappingWeightsAreEnergyNormalized(t *testing.T) {
	renderer, err := NewRenderer(Standard514(), Config{})
	if err != nil {
		t.Fatal(err)
	}
	position := spherical(-20, 20)
	mix, err := renderer.Render(Scene{Objects: []Object{{
		ID: "overlap", Position: position, Gain: 1,
	}}})
	if err != nil {
		t.Fatal(err)
	}
	source := mix.Sources[0]
	if len(source.Contributions) < 3 {
		t.Fatalf("active speaker count = %d, want overlapping pan", len(source.Contributions))
	}
	requireClose(t, "panning energy", contributionEnergy(source.Contributions), 1)
	requireClose(t, "distance", source.Distance, 1)

	sum := 0.0
	for _, contribution := range source.Contributions {
		sum += contribution.Gain
	}
	if sum <= 1 {
		t.Fatalf("sum of overlapping amplitudes = %.6f, want > 1", sum)
	}
}

func requireClose(t *testing.T, label string, got, want float64) {
	t.Helper()
	if math.Abs(got-want) > 1e-7 {
		t.Fatalf("%s = %.12f, want %.12f", label, got, want)
	}
}

func requireVecClose(t *testing.T, label string, got, want Vec3) {
	t.Helper()
	requireClose(t, label+".X", got.X, want.X)
	requireClose(t, label+".Y", got.Y, want.Y)
	requireClose(t, label+".Z", got.Z, want.Z)
}

func contributionEnergy(contributions []Contribution) float64 {
	energy := 0.0
	for _, contribution := range contributions {
		energy += contribution.Gain * contribution.Gain
	}
	return math.Sqrt(energy)
}

func channelEnergy(source SourceResult, names ...string) float64 {
	energy := 0.0
	for _, name := range names {
		for _, contribution := range source.Contributions {
			if contribution.Speaker == name {
				energy += contribution.Gain * contribution.Gain
			}
		}
	}
	return math.Sqrt(energy)
}

func contributionMap(source SourceResult) map[string]float64 {
	result := make(map[string]float64, len(source.Contributions))
	for _, contribution := range source.Contributions {
		result[contribution.Speaker] = contribution.Gain
	}
	return result
}
