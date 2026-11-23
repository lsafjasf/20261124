package ac3

import (
	"os"
	"path/filepath"
	"testing"
)

func loadTestStream(t *testing.T, name string) *Stream {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", "testdata", name))
	if err != nil {
		t.Skipf("testdata/%s not present: %v", name, err)
	}
	stream, err := ParseStream(raw)
	if err != nil {
		t.Fatalf("ParseStream(%s): %v", name, err)
	}
	return stream
}

func TestParseRealStreams(t *testing.T) {
	cases := map[string]int{
		"stereo192.ac3": 63,
		"sixch448.ac3":  63,
		"mono128.ac3":   58,
	}
	for name, wantFrames := range cases {
		stream := loadTestStream(t, name)
		if len(stream.Frames) != wantFrames {
			t.Errorf("%s: got %d frames, want %d", name, len(stream.Frames), wantFrames)
		}
	}
}

func TestSetDialnormRoundTripOnRealStreams(t *testing.T) {
	for _, name := range []string{"stereo192.ac3", "sixch448.ac3", "mono128.ac3"} {
		stream := loadTestStream(t, name)
		for _, frame := range stream.Frames {
			target := (frame.Dialnorm + 7) % 32
			before := append([]byte(nil), frame.Data...)
			changed, err := SetDialnorm(frame, target)
			if err != nil {
				t.Fatalf("%s frame %d: %v", name, frame.Index, err)
			}
			if !changed {
				t.Fatalf("%s frame %d: expected change", name, frame.Index)
			}
			if frame.Dialnorm != target {
				t.Fatalf("%s frame %d: dialnorm = %d, want %d", name, frame.Index, frame.Dialnorm, target)
			}
			// Post-edit CRC must be zero residue.
			r1, r2 := crcResidues(frame.Data)
			if r1 != 0 || r2 != 0 {
				t.Fatalf("%s frame %d: CRC residue %04X/%04X", name, frame.Index, r1, r2)
			}
			// Frame size is preserved.
			if len(before) != len(frame.Data) {
				t.Fatalf("%s frame %d: length changed", name, frame.Index)
			}
		}
		// Re-parsing the whole rewritten stream must succeed.
		if _, err := ParseStream(stream.Bytes()); err != nil {
			t.Fatalf("%s: reparsed stream invalid: %v", name, err)
		}
	}
}

func TestSetDialnormRejectsOutOfRange(t *testing.T) {
	stream := loadTestStream(t, "stereo192.ac3")
	if _, err := SetDialnorm(stream.Frames[0], 32); err == nil {
		t.Fatal("expected error for dialnorm 32")
	}
	if _, err := SetDialnorm(stream.Frames[0], -1); err == nil {
		t.Fatal("expected error for dialnorm -1")
	}
}

func TestNoOpEditKeepsFrameIdentical(t *testing.T) {
	stream := loadTestStream(t, "stereo192.ac3")
	frame := stream.Frames[0]
	before := append([]byte(nil), frame.Data...)
	changed, err := SetDialnorm(frame, frame.Dialnorm)
	if err != nil {
		t.Fatal(err)
	}
	if changed {
		t.Fatal("identical dialnorm reported as changed")
	}
	if payloadHash(before) != payloadHash(frame.Data) {
		t.Fatal("no-op edit altered frame bytes")
	}
}
