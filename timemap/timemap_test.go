package timemap

import (
	"math/big"
	"testing"
)

func baseSpec24to30(frames int64, kfs []Keyframe) Spec {
	return Spec{
		SourceRate:   Rate24,
		SourceFrames: frames,
		AudioRate:    Rate48000,
		OutputRate:   Rate30,
		OutputFrames: 60,
		Keyframes:    kfs,
	}
}

func TestForward24To30(t *testing.T) {
	// 1x forward over 2 seconds; output runs 0..59 frames (2s).
	s := baseSpec24to30(48, []Keyframe{
		KF(0, 1, 0, 1),
		KF(2, 1, 2, 1),
	})
	tbl, err := Build(s)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if tbl.Len() != 60 {
		t.Fatalf("Len = %d, want 60", tbl.Len())
	}

	wantFrame := map[int64]int64{
		0: 0, 4: 3, 5: 4, 29: 23, 30: 24, 59: 47,
	}
	for n, want := range wantFrame {
		got, err := tbl.SourceFrame(n)
		if err != nil {
			t.Fatalf("SourceFrame(%d): %v", n, err)
		}
		if got != want {
			t.Errorf("SourceFrame(%d) = %d, want %d", n, got, want)
		}
	}

	// Exact audio sample for frame 10: source time = 10/30 s * 1x.
	row, err := tbl.Row(10)
	if err != nil {
		t.Fatal(err)
	}
	wantSample := int64(16000)
	if row.AudioSample != wantSample {
		t.Errorf("AudioSample(10) = %d, want %d", row.AudioSample, wantSample)
	}
	wantTime := big.NewRat(1, 3)
	if row.SourceTime.Cmp(wantTime) != 0 {
		t.Errorf("SourceTime(10) = %s, want %s", row.SourceTime, wantTime)
	}

	assertNonDecreasingFrames(t, tbl)
}

func TestFreeze24To30(t *testing.T) {
	// Freeze at the open end of the profile is legal: hold source t=1s
	// for the whole second output second.
	s := baseSpec24to30(48, []Keyframe{
		KF(0, 1, 1, 1),
		KF(2, 1, 1, 1),
	})
	tbl, err := Build(s)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	for n := int64(30); n < 60; n++ {
		got, _ := tbl.SourceFrame(n)
		if got != 24 {
			t.Fatalf("frozen SourceFrame(%d) = %d, want 24", n, got)
		}
		sample, _ := tbl.AudioSample(n)
		if sample != 48000 {
			t.Fatalf("frozen AudioSample(%d) = %d, want 48000", n, sample)
		}
	}
}

func TestReverse24To30(t *testing.T) {
	// 1x reverse from source t=1s back to t=0 over one output second.
	s := baseSpec24to30(48, []Keyframe{
		KF(0, 1, 1, 1),
		KF(1, 1, 0, 1),
	})
	s.OutputFrames = 30
	tbl, err := Build(s)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	wantFrame := map[int64]int64{0: 24, 1: 23, 29: 0}
	for n, want := range wantFrame {
		got, _ := tbl.SourceFrame(n)
		if got != want {
			t.Errorf("SourceFrame(%d) = %d, want %d", n, got, want)
		}
	}
	assertNonIncreasingFrames(t, tbl)
}

func TestSpeedStep24To30(t *testing.T) {
	// 1x for the first output second then 2x for the second, with a
	// continuous position at the boundary.
	s := baseSpec24to30(72, []Keyframe{
		KF(0, 1, 0, 1),
		KF(1, 1, 1, 1),
		KF(2, 1, 3, 1),
	})
	tbl, err := Build(s)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	// Frame 30 sits on the boundary: source time exactly 1s -> frame 24.
	if got, _ := tbl.SourceFrame(30); got != 24 {
		t.Fatalf("boundary SourceFrame(30) = %d, want 24", got)
	}
	// Frame 45: output t=1.5s, source = 1 + 2*0.5 = 2s -> frame 48.
	if got, _ := tbl.SourceFrame(45); got != 48 {
		t.Fatalf("SourceFrame(45) = %d, want 48", got)
	}
	assertNonDecreasingFrames(t, tbl)
}

func assertNonDecreasingFrames(t *testing.T, tbl *Table) {
	t.Helper()
	for n := 1; n < tbl.Len(); n++ {
		a := tbl.Rows[n-1].SourceFrame
		b := tbl.Rows[n].SourceFrame
		if b < a {
			t.Fatalf("frames decrease at %d: %d -> %d", n, a, b)
		}
	}
}

func assertNonIncreasingFrames(t *testing.T, tbl *Table) {
	t.Helper()
	for n := 1; n < tbl.Len(); n++ {
		a := tbl.Rows[n-1].SourceFrame
		b := tbl.Rows[n].SourceFrame
		if b > a {
			t.Fatalf("frames increase at %d: %d -> %d", n, a, b)
		}
	}
}
