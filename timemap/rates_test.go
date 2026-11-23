package timemap

import (
	"math/big"
	"testing"
	"time"
)

func exactSourceTime(n int64, outRate, speed *big.Rat) *big.Rat {
	tn := new(big.Rat).Quo(big.NewRat(n, 1), outRate)
	return new(big.Rat).Mul(tn, speed)
}

func exactSourceFrame(t *testing.T, srcTime, srcRate *big.Rat, frames int64) int64 {
	t.Helper()
	r := new(big.Rat).Mul(srcTime, srcRate)
	f := new(big.Int).Quo(r.Num(), r.Denom())
	if !f.IsInt64() {
		t.Fatalf("source frame not int64: %s", r)
	}
	v := f.Int64()
	if v == frames {
		v--
	}
	return v
}

func Test2997ExactForward(t *testing.T) {
	// 10 seconds of 24fps source, output at exactly 30000/1001 fps.
	const seconds = int64(10)
	outFrames := seconds * 30 // 300 output frame slots
	s := Spec{
		SourceRate:   Rate24,
		SourceFrames: 24 * seconds,
		AudioRate:    Rate48000,
		OutputRate:   Rate2997,
		OutputFrames: outFrames,
		Keyframes: []Keyframe{
			KF(0, 1, 0, 1),
			KF(seconds, 1, seconds, 1),
		},
	}
	tbl, err := Build(s)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if tbl.Len() != int(outFrames) {
		t.Fatalf("Len = %d, want %d", tbl.Len(), outFrames)
	}

	for _, n := range []int64{0, 1, 29, 30, 100, 299} {
		row, err := tbl.Row(n)
		if err != nil {
			t.Fatal(err)
		}
		wantTime := exactSourceTime(n, Rate2997, big.NewRat(1, 1))
		if row.SourceTime.Cmp(wantTime) != 0 {
			t.Errorf("frame %d SourceTime = %s, want %s", n, row.SourceTime, wantTime)
		}
		wantFrame := exactSourceFrame(t, wantTime, Rate24, s.SourceFrames)
		if row.SourceFrame != wantFrame {
			t.Errorf("frame %d SourceFrame = %d, want %d", n, row.SourceFrame, wantFrame)
		}
		wantAudio := new(big.Rat).Mul(wantTime, Rate48000)
		if row.AudioPosition.Cmp(wantAudio) != 0 {
			t.Errorf("frame %d AudioPosition = %s, want %s", n, row.AudioPosition, wantAudio)
		}
	}
}

func Test2997MixedWarp(t *testing.T) {
	// Forward 1x then reverse 1x back to start, spanning a 29.97 output.
	s := Spec{
		SourceRate:   Rate24,
		SourceFrames: 72,
		AudioRate:    Rate48000,
		OutputRate:   Rate2997,
		OutputFrames: 59,
		Keyframes: []Keyframe{
			KF(0, 1, 0, 1),
			KF(1, 1, 1, 1),
			KF(2, 1, 0, 1),
		},
	}
	tbl, err := Build(s)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	// Frame 30 is just past the 1s boundary under 30000/1001, so it must
	// already be travelling backward, never beyond source frame 24.
	if got, _ := tbl.SourceFrame(29); got > 24 {
		t.Fatalf("SourceFrame(29) = %d, want <= 24", got)
	}
	if got, _ := tbl.SourceFrame(30); got >= 24 {
		t.Fatalf("SourceFrame(30) = %d, want < 24 (reverse leg)", got)
	}

	// Reverse leg: last output frame maps back to source frame 0.
	last := tbl.Rows[len(tbl.Rows)-1]
	if last.SourceFrame > 1 {
		t.Fatalf("last SourceFrame = %d, want 0 or 1", last.SourceFrame)
	}
}

func TestLongClip24To30(t *testing.T) {
	const minutes = 60
	seconds := int64(minutes * 60)
	s := Spec{
		SourceRate:   Rate24,
		SourceFrames: 24 * seconds,
		AudioRate:    Rate48000,
		OutputRate:   Rate30,
		OutputFrames: 30 * seconds,
		Keyframes: []Keyframe{
			KF(0, 1, 0, 1),
			KF(seconds, 1, seconds, 1),
		},
	}

	start := time.Now()
	tbl, err := Build(s)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	elapsed := time.Since(start)
	if tbl.Len() != int(s.OutputFrames) {
		t.Fatalf("Len = %d, want %d", tbl.Len(), s.OutputFrames)
	}

	// Boundary rows must stay inside the clip.
	first, last := tbl.Rows[0], tbl.Rows[tbl.Len()-1]
	if first.SourceFrame != 0 || last.SourceFrame != s.SourceFrames-1 {
		t.Fatalf("boundary frames = %d..%d, want 0..%d",
			first.SourceFrame, last.SourceFrame, s.SourceFrames-1)
	}
	wantLastTime := new(big.Rat).Sub(
		big.NewRat(seconds, 1),
		new(big.Rat).Inv(Rate30),
	)
	if last.SourceTime.Cmp(wantLastTime) != 0 {
		t.Fatalf("last SourceTime = %s, want %s", last.SourceTime, wantLastTime)
	}
	wantLastSampleRat := new(big.Rat).Mul(wantLastTime, Rate48000)
	wantLastSample := new(big.Int).Quo(wantLastSampleRat.Num(), wantLastSampleRat.Denom())
	if last.AudioSample != wantLastSample.Int64() {
		t.Fatalf("last AudioSample = %d, want %s", last.AudioSample, wantLastSample)
	}

	// Exact spot check near the middle and the end.
	mid, err := tbl.Row(s.OutputFrames / 2)
	if err != nil {
		t.Fatal(err)
	}
	wantMidTime := big.NewRat(seconds, 2)
	if mid.SourceTime.Cmp(wantMidTime) != 0 {
		t.Errorf("mid SourceTime = %s, want %s", mid.SourceTime, wantMidTime)
	}

	t.Logf("long clip: %d rows built in %s", tbl.Len(), elapsed)
}

func TestLongClip2997(t *testing.T) {
	const minutes = 10
	seconds := int64(minutes * 60)
	s := Spec{
		SourceRate:   Rate24,
		SourceFrames: 24 * seconds,
		AudioRate:    Rate48000,
		OutputRate:   Rate2997,
		OutputFrames: Rate2997.Num().Int64() * seconds / Rate2997.Denom().Int64(), // floor(29.97*600)
		Keyframes: []Keyframe{
			KF(0, 1, 0, 1),
			KF(seconds, 1, seconds, 1),
		},
	}
	tbl, err := Build(s)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if tbl.Len() != int(s.OutputFrames) {
		t.Fatalf("Len = %d, want %d", tbl.Len(), s.OutputFrames)
	}
	last := tbl.Rows[tbl.Len()-1]
	if last.SourceFrame < 0 || last.SourceFrame >= s.SourceFrames {
		t.Fatalf("last SourceFrame %d out of bounds (0..%d)",
			last.SourceFrame, s.SourceFrames-1)
	}
	if last.AudioSample < 0 {
		t.Fatalf("last AudioSample negative: %d", last.AudioSample)
	}
}

func TestDeterministicAcrossBuilds(t *testing.T) {
	s := Spec{
		SourceRate:   Rate2997,
		SourceFrames: 120,
		AudioRate:    Rate48000,
		OutputRate:   Rate30,
		OutputFrames: 120,
		Keyframes: []Keyframe{
			KF(0, 1, 0, 1),
			KF(2, 1, 1, 1),
			KF(4, 1, 0, 1),
		},
	}
	first, err := Build(s)
	if err != nil {
		t.Fatal(err)
	}
	second, err := Build(s)
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Rows) != len(second.Rows) {
		t.Fatal("row count differs between builds")
	}
	for i := range first.Rows {
		a, b := first.Rows[i], second.Rows[i]
		if a.OutputFrame != b.OutputFrame ||
			a.SourceFrame != b.SourceFrame ||
			a.AudioSample != b.AudioSample ||
			a.SourceTime.Cmp(b.SourceTime) != 0 ||
			a.AudioPosition.Cmp(b.AudioPosition) != 0 ||
			a.OutputTime.Cmp(b.OutputTime) != 0 {
			t.Fatalf("row %d differs between builds:\n%+v\n%+v", i, a, b)
		}
	}
}
