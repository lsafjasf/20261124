package inspect

import (
	"bytes"
	"testing"

	"vxcheck/internal/fixture"
	"vxcheck/internal/vx"
)

func buildPair(t *testing.T) (*vx.File, *vx.File) {
	t.Helper()
	spec := fixture.Default()
	var sdrBuf, hdrBuf bytes.Buffer
	if err := fixture.WriteSDR(&sdrBuf, spec); err != nil {
		t.Fatal(err)
	}
	if err := fixture.WriteHDR(&hdrBuf, spec); err != nil {
		t.Fatal(err)
	}
	sdr, err := vx.Read(bytes.NewReader(sdrBuf.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	hdr, err := vx.Read(bytes.NewReader(hdrBuf.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	return sdr, hdr
}

func TestRunDetectsAllDefects(t *testing.T) {
	sdr, hdr := buildPair(t)
	report := Run(sdr, hdr)

	wantKinds := map[string]bool{
		"frame-dropped-in-hdr":        false,
		"frame-number-mismatch":       false,
		"color-metadata-inconsistent": false,
		"subtitle-boundary-shift":     false,
		"hdr-audio-gap":               false,
		"audio-sample-divergence":     false,
	}
	for _, f := range report.Findings {
		if _, ok := wantKinds[f.Kind]; ok {
			wantKinds[f.Kind] = true
		}
	}
	for kind, found := range wantKinds {
		if !found {
			t.Errorf("expected finding %q", kind)
		}
	}

	if report.FirstTimeline == nil {
		t.Fatal("expected first timeline anomaly")
	}
	if report.FirstTimeline.Kind != "subtitle-boundary-shift" {
		t.Fatalf("unexpected first anomaly: %s", report.FirstTimeline.Kind)
	}
	if len(report.HeaderAnomalies) == 0 ||
		report.HeaderAnomalies[0].Kind != "color-metadata-inconsistent" {
		t.Fatalf("header anomaly not separated: %+v", report.HeaderAnomalies)
	}
}

func TestAlignmentMapsDrop(t *testing.T) {
	sdr, hdr := buildPair(t)
	width := sdr.Header.Width
	sf := BuildFeatures(sdr, width, sdr.Header.Height)
	hf := BuildFeatures(hdr, width, hdr.Header.Height)
	al := Align4(sf, hf, width)

	if len(al.Pairs) != 95 {
		t.Fatalf("matched pairs = %d, want 95", len(al.Pairs))
	}
	if len(al.SDROnly) != 1 || al.SDROnly[0] != 41 {
		t.Fatalf("SDR-only frames = %v, want [41]", al.SDROnly)
	}
	// After the drop, SDR 42 must align to HDR stored frame 41 (PTS 42).
	last := al.Pairs[len(al.Pairs)-1]
	if last.SDRIndex != 95 || last.HDRIndex != 94 {
		t.Fatalf("tail pair = (%d,%d), want (95,94)", last.SDRIndex, last.HDRIndex)
	}
}

func TestToneClassifiedSeparately(t *testing.T) {
	sdr, hdr := buildPair(t)
	report := Run(sdr, hdr)

	normal := 0
	for _, f := range report.Findings {
		if f.Severity == SevNormal {
			normal++
			if f.Kind != "tone-difference" {
				t.Fatalf("normal finding has unexpected kind %s", f.Kind)
			}
		}
	}
	if normal != 95 {
		t.Fatalf("normal tone findings = %d, want 95", normal)
	}
	if report.Tone.MatchedPairs != 95 || report.Tone.MaxEdgeDiff > matchLimit4 {
		t.Fatalf("tone stats wrong: %+v", report.Tone)
	}
	// All matched pairs must preserve the scene-cut flag position.
	cuts := map[int64]bool{}
	for _, c := range Cuts(hdr) {
		cuts[c.Tick] = true
	}
	for _, c := range Cuts(sdr) {
		if !cuts[c.Tick] {
			t.Fatalf("cut tick %d lost in HDR", c.Tick)
		}
	}
}

func TestAudioGapDetails(t *testing.T) {
	sdr, hdr := buildPair(t)
	findings := CheckAudio(sdr, hdr)
	var gap *Finding
	var divergence *Finding
	for i := range findings {
		switch findings[i].Kind {
		case "hdr-audio-gap":
			gap = &findings[i]
		case "audio-sample-divergence":
			divergence = &findings[i]
		}
	}
	if gap == nil || divergence == nil {
		t.Fatalf("missing audio findings: %+v", findings)
	}
	// 48 kHz / 24 fps = 2000 samples per frame; first gap at frame 41.
	if gap.HDRSec != float64(84000)/48000 {
		t.Fatalf("gap at %v, want 1.75 s", gap.HDRSec)
	}
	if !contains(divergence.Detail, "position 82000") {
		t.Fatalf("divergence detail: %s", divergence.Detail)
	}
}

func contains(s, sub string) bool {
	return bytes.Contains([]byte(s), []byte(sub))
}

func TestCleanPairHasNoDefects(t *testing.T) {
	sdr, _ := buildPair(t)
	report := Run(sdr, sdr)

	for _, f := range report.Findings {
		if f.Severity == SevDefect || f.Severity == SevMeta {
			t.Fatalf("unexpected finding on clean pair: %+v", f)
		}
	}
	if report.FirstAnomaly != nil || report.FirstTimeline != nil {
		t.Fatal("clean pair must have no anomaly")
	}
	if len(report.HeaderAnomalies) != 0 {
		t.Fatalf("clean pair header anomalies: %+v", report.HeaderAnomalies)
	}
	if report.Tone.MatchedPairs != len(sdr.Frames) {
		t.Fatalf("matched %d, want %d", report.Tone.MatchedPairs, len(sdr.Frames))
	}
}
