package audio

import (
	"path/filepath"
	"testing"
)

func TestWAVRoundTrip(t *testing.T) {
	src := GenerateSpeechLike(48000, 4800, 11)
	path := filepath.Join(t.TempDir(), "tone.wav")
	if err := WriteWAV16(path, 48000, src); err != nil {
		t.Fatal(err)
	}
	rate, got, err := ReadWAV16(path)
	if err != nil {
		t.Fatal(err)
	}
	if rate != 48000 || len(got) != len(src) {
		t.Fatalf("rate=%d len=%d want len=%d", rate, len(got), len(src))
	}
	maxErr := 0.0
	for i := range src {
		if d := abs(got[i] - src[i]); d > maxErr {
			maxErr = d
		}
	}
	if maxErr > 1.0/32768.0*2 {
		t.Fatalf("round-trip error too large: %f", maxErr)
	}
}

func TestFramesFlatten(t *testing.T) {
	src := make([]float64, 1000)
	for i := range src {
		src[i] = float64(i)
	}
	frames := Frames(src, 96)
	if len(frames) != 10 {
		t.Fatalf("frames = %d", len(frames))
	}
	flat := Flatten(frames)
	if len(flat) != 960 || flat[959] != 959 {
		t.Fatal("flatten mismatch")
	}
}

func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}
