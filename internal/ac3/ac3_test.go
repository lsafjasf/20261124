package ac3

import (
	"bytes"
	"testing"
)

func TestFrameTableKnownSizes(t *testing.T) {
	cases := []struct {
		code, fscod, wantWords int
	}{
		{20, 0, 384}, // 192 kbps @ 48 kHz -> 768 bytes
		{30, 0, 896}, // 448 kbps @ 48 kHz -> 1792 bytes
		{16, 1, 278}, // 128 kbps @ 44.1 kHz even word count
		{17, 1, 279}, // paired odd code, one extra padding word
		{6, 1, 121},  // 56 kbps @ 44.1 kHz
	}
	for _, tc := range cases {
		if got := frameSizeWords[tc.code][tc.fscod]; got != tc.wantWords {
			t.Errorf("frameSizeWords[%d][%d] = %d, want %d", tc.code, tc.fscod, got, tc.wantWords)
		}
		if got := frameSizeBytes(tc.code, tc.fscod); got != 2*tc.wantWords {
			t.Errorf("frameSizeBytes(%d,%d) = %d, want %d", tc.code, tc.fscod, got, 2*tc.wantWords)
		}
	}
}

func TestCRCSplitUsesEvenWordPair(t *testing.T) {
	// 278 and 279 word frames (556/558 bytes) must share the split at byte 346.
	if got := crcSplitByte(556); got != 346 {
		t.Errorf("crcSplitByte(556) = %d, want 346", got)
	}
	if got := crcSplitByte(558); got != 346 {
		t.Errorf("crcSplitByte(558) = %d, want 346", got)
	}
	if got := crcSplitByte(768); got != 480 {
		t.Errorf("crcSplitByte(768) = %d, want 480", got)
	}
}

func TestCRCResidueAndRewriteRoundTrip(t *testing.T) {
	frame := make([]byte, 768)
	frame[0], frame[1] = 0x0B, 0x77
	// Random-ish payload that still leaves the frame parseable shape; only the
	// CRC machinery is under test here.
	for i := 4; i < 768; i++ {
		frame[i] = byte((i * 37) % 251)
	}
	c1, c2 := frameCRCs(frame)
	frame[2], frame[3] = byte(c1>>8), byte(c1)
	frame[766], frame[767] = byte(c2>>8), byte(c2)

	r1, r2 := crcResidues(frame)
	if r1 != 0 || r2 != 0 {
		t.Fatalf("rewritten CRC not self-consistent: r1=%04X r2=%04X", r1, r2)
	}

	// A second recomputation must be stable (idempotent rewrite).
	c1b, c2b := frameCRCs(frame)
	if c1b != c1 || c2b != c2 {
		t.Fatalf("CRC rewrite not stable: %04X/%04X vs %04X/%04X", c1, c2, c1b, c2b)
	}
}

func TestWriteBitsIsBitAccurate(t *testing.T) {
	data := make([]byte, 4)
	if err := writeBits(data, 5, 5, 27); err != nil {
		t.Fatal(err)
	}
	// 27 = 11011 placed at bit 5: byte0 lower 3 bits = 110, byte1 upper 2 = 11
	if data[0] != 0b00000110 || data[1] != 0b11000000 {
		t.Fatalf("straddling writeBits wrong: %08b %08b", data[0], data[1])
	}
	got, err := readBitsAt(data, 5, 5)
	if err != nil || got != 27 {
		t.Fatalf("readBitsAt = %d, %v; want 27", got, err)
	}
}

func TestFramesEqual(t *testing.T) {
	a := &Frame{Data: []byte{1, 2, 3}}
	b := &Frame{Data: []byte{1, 2, 3}}
	c := &Frame{Data: []byte{1, 2, 4}}
	if !FramesEqual(a, b) {
		t.Error("identical frames reported unequal")
	}
	if FramesEqual(a, c) {
		t.Error("different frames reported equal")
	}
	if !bytes.Equal(a.Data, b.Data) {
		t.Fatal("test setup altered source data")
	}
}
