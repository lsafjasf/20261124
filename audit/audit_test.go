package audit

import (
	"math/big"
	"os"
	"path/filepath"
	"testing"

	"subdrift/subtitle"
)

func loadSample(t *testing.T) *subtitle.Project {
	t.Helper()
	p, err := subtitle.LoadProject(filepath.Join("..", "testdata", "reel_500s.json"))
	if err != nil {
		t.Fatalf("load sample: %v", err)
	}
	return p
}

func TestFirstErrorEventIsRateReinterpretation(t *testing.T) {
	p := loadSample(t)
	r, err := Run(p)
	if err != nil {
		t.Fatal(err)
	}
	if r.FirstError == nil {
		t.Fatal("expected an error event")
	}
	got := r.FirstError
	if got.Frame != 24 || got.CueID != "c001" || got.Endpoint != "end" {
		t.Fatalf("first error = %s %s frame %d, want c001 end frame 24", got.CueID, got.Endpoint, got.Frame)
	}
	if got.ReferenceMS != 1001 || got.OutputMS != 1000 || got.ErrorMS != -1 {
		t.Fatalf("first error output=%d ref=%d err=%d, want 1000/1001/-1", got.OutputMS, got.ReferenceMS, got.ErrorMS)
	}
	if got.FirstStage != 0 {
		t.Fatalf("first fault stage = %d, want 0 (frame->pts)", got.FirstStage)
	}
	stage := got.Stages[0]
	if stage.Output.Count != 90000 {
		t.Fatalf("legacy PTS = %d, want 90000", stage.Output.Count)
	}
	if stage.Expected.RatString() != "90090" {
		t.Fatalf("expected PTS = %s, want 90090", stage.Expected.RatString())
	}
	if stage.CumulativeErrorMS.RatString() != "-1" {
		t.Fatalf("stage error = %s ms, want -1", stage.CumulativeErrorMS.RatString())
	}
}

func TestEndOfReelIsHalfSecondEarly(t *testing.T) {
	p := loadSample(t)
	r, err := Run(p)
	if err != nil {
		t.Fatal(err)
	}
	last := r.Boundaries[len(r.Boundaries)-1]
	if last.Frame != 11988 {
		t.Fatalf("last frame = %d, want 11988", last.Frame)
	}
	if last.ReferenceMS != 500000 || last.OutputMS != 499500 || last.ErrorMS != -500 {
		t.Fatalf("end cue output=%d ref=%d err=%d, want 499500/500000/-500",
			last.OutputMS, last.ReferenceMS, last.ErrorMS)
	}
	if got := last.GroundMS.FloatString(3); got != "499999.500" {
		t.Fatalf("ground ms = %s, want 499999.500", got)
	}
	if got := last.ErrorFrames.FloatString(6); got != "-11.988012" {
		t.Fatalf("error frames = %s, want -11.988012", got)
	}
}

func TestFixedPathHasNoDrift(t *testing.T) {
	p := loadSample(t)
	mode := ReferenceMode(p)
	maxErr := int64(0)
	for frame := int64(0); frame <= p.DurationFrames(); frame++ {
		b := Evaluate(frame, p, mode)
		if abs(b.ErrorMS) > maxErr {
			maxErr = abs(b.ErrorMS)
		}
	}
	for _, frame := range []int64{0, 24, 240, 2400, 5994, 11988} {
		b := Evaluate(frame, p, mode)
		if b.ErrorMS < -1 || b.ErrorMS > 1 {
			t.Fatalf("fixed frame %d error = %d ms, want within +-1 ms", frame, b.ErrorMS)
		}
	}
	if maxErr > 1 {
		t.Fatalf("fixed all-frame max error = %d ms, want <= 1", maxErr)
	}
	last := Evaluate(11988, p, mode)
	if last.OutputMS != 500000 {
		t.Fatalf("fixed last ms = %d, want 500000", last.OutputMS)
	}
}

func TestLayerIsolation(t *testing.T) {
	p := loadSample(t)
	last := p.DurationFrames()
	want := map[string]int64{
		"nominal-rate-only":   -500,
		"timebase-floor-only": -1,
		"rounding-cs-only":    -10,
		"full-legacy":         -500,
	}
	for _, v := range variantModes(p) {
		b := Evaluate(last, p, v.mode)
		if b.ErrorMS != want[v.name] {
			t.Fatalf("%s error = %d ms, want %d", v.name, b.ErrorMS, want[v.name])
		}
	}
}

func TestGlobalOffsetDoesNotFixEarlyCues(t *testing.T) {
	p := loadSample(t)
	r, err := Run(p)
	if err != nil {
		t.Fatal(err)
	}
	// A naive +500 ms scalar offset zeroes the final sample but turns the
	// first one-millisecond-early cue into a 499 ms late cue.
	first := r.FirstError
	if first == nil {
		t.Fatal("expected first error")
	}
	if shifted := first.ErrorMS + 500; shifted != 499 {
		t.Fatalf("globally shifted first cue error = %d ms, want 499", shifted)
	}
	// The fixed curve must remain bounded at both ends without a scalar offset.
	fixed := ReferenceMode(p)
	for _, frame := range []int64{24, 11988} {
		if got := abs(Evaluate(frame, p, fixed).ErrorMS); got > 1 {
			t.Fatalf("fixed frame %d error = %d ms, want <= 1", frame, got)
		}
	}
}

func TestReportCurveEndpoint(t *testing.T) {
	p := loadSample(t)
	r, err := Run(p)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Curve) < 100 {
		t.Fatalf("curve has %d points, want a dense sweep", len(r.Curve))
	}
	first, last := r.Curve[0], r.Curve[len(r.Curve)-1]
	if first.ErrorMS != 0 {
		t.Fatalf("curve starts at %d ms, want 0", first.ErrorMS)
	}
	if last.ErrorMS != -500 {
		t.Fatalf("curve ends at %d ms, want -500", last.ErrorMS)
	}
}

func TestErrorFormulaAtCheckpoints(t *testing.T) {
	p := loadSample(t)
	for _, tc := range []struct {
		frame int64
		want  int64
	}{
		{24, -1},
		{240, -10},
		{2400, -100},
		{5994, -250},
		{11988, -500},
	} {
		b := Evaluate(tc.frame, p, LegacyMode(p))
		if b.ErrorMS != tc.want {
			t.Fatalf("frame %d error = %d ms, want %d", tc.frame, b.ErrorMS, tc.want)
		}
		// Nominal-rate error alone explains the endpoint: -frame/24 ms.
		formula := new(big.Rat).SetFrac64(-tc.frame, 24)
		diff := new(big.Rat).Sub(new(big.Rat).SetInt64(tc.want), formula)
		if diff.Sign() != 0 {
			// The centisecond floor may add up to 10 ms; the 250/500 ms
			// checkpoints should be exact.
			if tc.frame == 24 || tc.frame == 240 || tc.frame == 2400 {
				t.Fatalf("frame %d differs from -frame/24 by %s ms", tc.frame, diff.RatString())
			}
		}
	}
}

func TestSRTRegressionGolden(t *testing.T) {
	p := loadSample(t)
	for _, tc := range []struct {
		name string
		mode Mode
		file string
	}{
		{"legacy", LegacyMode(p), "reel_500s_legacy.srt"},
		{"fixed", ReferenceMode(p), "reel_500s_fixed.srt"},
	} {
		got, err := RenderSRTString(p, tc.mode)
		if err != nil {
			t.Fatal(err)
		}
		want, err := os.ReadFile(filepath.Join("..", "testdata", tc.file))
		if err != nil {
			t.Fatal(err)
		}
		if got != string(want) {
			t.Fatalf("%s golden mismatch:\n--- got ---\n%s--- want ---\n%s", tc.name, got, want)
		}
	}
}
