package main

import (
	"math"
	"testing"
)

// goldenSkin is the known single-tone-map result for the skin patch,
// captured from the reference chain (exposure -> one tone map -> sRGB).
var goldenSkin = [3]float64{
	0.59758944845098627,
	0.51819741019384902,
	0.47500080381254695,
}

func TestFixedPipelineMatchesGolden(t *testing.T) {
	fixed := &Pipeline{Buggy: false}
	dumps, golden := fixed.RunTimeline()
	for i, d := range dumps {
		if rms := frameRMSErr(d.Stages["encode"], golden[i]); rms != 0 {
			t.Errorf("frame %d: fixed pipeline RMS vs golden = %g, want 0", i, rms)
		}
		m := SkinPatchMean(d.Stages["encode"])
		for c := 0; c < 3; c++ {
			if math.Abs(m[c]-goldenSkin[c]) > 1e-12 {
				t.Errorf("frame %d: skin[%d] = %.15g, want %.15g", i, c, m[c], goldenSkin[c])
			}
		}
	}
}

func TestBuggyPipelineShowsDefectOnBrightFrames(t *testing.T) {
	buggy := &Pipeline{Buggy: true}
	dumps, golden := buggy.RunTimeline()
	for i, d := range dumps {
		rms := frameRMSErr(d.Stages["encode"], golden[i])
		bright := Shots()[i].KeyLight > 1.0
		if bright && rms < 1e-3 {
			t.Errorf("frame %d (bright): buggy RMS vs golden = %g, want significant deviation", i, rms)
		}
		if !bright && rms != 0 {
			t.Errorf("frame %d (normal): buggy RMS vs golden = %g, want 0", i, rms)
		}
	}
}

func TestDiagnoseLocalizesSecondToneMap(t *testing.T) {
	buggy := &Pipeline{Buggy: true}
	dumps, _ := buggy.RunTimeline()
	for i, d := range dumps {
		fd := DiagnoseFrame(d)
		bright := Shots()[i].KeyLight > 1.0
		got := len(fd.ExtraTMAt) > 0
		if bright != got {
			t.Errorf("frame %d: extra tone map detected = %v, want %v (%v)", i, got, bright, fd.ExtraTMAt)
		}
		if got && fd.ExtraTMAt[0] != "tonemap->look" {
			t.Errorf("frame %d: extra tone map at %q, want tonemap->look", i, fd.ExtraTMAt[0])
		}
		// exposure stage must always be a pure linear gain
		if best := FitStage(d.Stages["scene"], d.Stages["exposure"])[0]; best.RMS > 1e-9 {
			t.Errorf("frame %d: exposure stage best fit %s rms=%g, want linear", i, best.Hypothesis, best.RMS)
		}
	}
}

func TestToneMapInverseRoundTrip(t *testing.T) {
	for _, x := range []float64{0.0, 0.01, 0.18, 0.5, 1.0, 4.0, 16.0, 100.0} {
		if got := ToneMapInverse(ToneMap(x)); math.Abs(got-x) > 1e-9*(1+x) {
			t.Errorf("ToneMapInverse(ToneMap(%g)) = %g", x, got)
		}
	}
}
