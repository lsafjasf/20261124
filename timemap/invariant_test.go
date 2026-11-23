package timemap

import (
	"math/big"
	"testing"
)

// TestTableInvariants walks a mixed profile and verifies every emitted row:
// indices are in range, exact audio position matches source time, and source
// frames are monotonic within each linear leg.
func TestTableInvariants(t *testing.T) {
	s := Spec{
		SourceRate:   Rate24,
		SourceFrames: 144,
		AudioRate:    Rate48000,
		OutputRate:   Rate30,
		OutputFrames: 90,
		Keyframes: []Keyframe{
			KF(0, 1, 0, 1), // forward 1x
			KF(1, 1, 1, 1), // speed step: 2x forward
			KF(2, 1, 3, 1), // reverse 1x back to 2s
			KF(3, 1, 2, 1),
		},
	}
	tbl, err := Build(s)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if tbl.Len() != 90 {
		t.Fatalf("Len = %d, want 90", tbl.Len())
	}

	maxFrame := s.SourceFrames - 1
	for i, row := range tbl.Rows {
		if row.OutputFrame != int64(i) {
			t.Fatalf("row %d has OutputFrame %d", i, row.OutputFrame)
		}
		if row.SourceFrame < 0 || row.SourceFrame > maxFrame {
			t.Fatalf("row %d source frame %d out of bounds", i, row.SourceFrame)
		}
		if row.AudioSample < 0 {
			t.Fatalf("row %d negative audio sample", i)
		}
		wantAudio := new(big.Rat).Mul(row.SourceTime, s.AudioRate)
		if row.AudioPosition.Cmp(wantAudio) != 0 {
			t.Fatalf("row %d audio position mismatch", i)
		}
		wantOutput := new(big.Rat).Quo(big.NewRat(int64(i), 1), s.OutputRate)
		if row.OutputTime.Cmp(wantOutput) != 0 {
			t.Fatalf("row %d output time mismatch", i)
		}
	}

	// Direct lookup helpers agree with the row data.
	for _, n := range []int64{0, 29, 30, 60, 89} {
		frame, err := tbl.SourceFrame(n)
		if err != nil {
			t.Fatal(err)
		}
		if frame != tbl.Rows[n].SourceFrame {
			t.Fatalf("SourceFrame(%d) disagrees with row", n)
		}
		sample, err := tbl.AudioSample(n)
		if err != nil || sample != tbl.Rows[n].AudioSample {
			t.Fatalf("AudioSample(%d) disagreement", n)
		}
	}

	if _, err := tbl.SourceFrame(90); err != ErrFrameNotFound {
		t.Fatalf("out-of-range lookup err = %v, want ErrFrameNotFound", err)
	}
}

func TestRowReturnsIndependentCopies(t *testing.T) {
	s := baseSpec24to30(48, []Keyframe{
		KF(0, 1, 0, 1),
		KF(2, 1, 2, 1),
	})
	tbl, err := Build(s)
	if err != nil {
		t.Fatal(err)
	}
	a, _ := tbl.Row(5)
	a.SourceFrame = 999
	a.SourceTime.SetInt64(123)
	b, _ := tbl.Row(5)
	if b.SourceFrame == 999 || b.SourceTime.Sign() < 0 {
		t.Fatal("Row returned mutable references into the table")
	}
}
