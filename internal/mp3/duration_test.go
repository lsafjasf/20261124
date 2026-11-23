package mp3

import (
	"encoding/binary"
	"testing"
)

// mpeg1Layer3Frame builds a 128 kbps / 44.1 kHz Layer III frame (418 bytes).
func mpeg1Frame(header uint32, size int) []byte {
	b := make([]byte, size)
	binary.BigEndian.PutUint32(b[0:4], header)
	// Keep the rest zero; byte 0xFF never appears, so no false syncs matter.
	return b
}

func TestScanDuration(t *testing.T) {
	// 0xFFFBA004: MPEG1, Layer III, 128 kbps, 44.1 kHz, no padding.
	const header = 0xfffba004
	fh, _ := parseHeader(header)
	frameSize := fh.length
	const samplesPerFrame = 1152
	var audio []byte
	for i := 0; i < 50; i++ {
		audio = append(audio, mpeg1Frame(header, frameSize)...)
	}
	ms, source := DurationMillis(audio, 0)
	if source != "frame-scan" {
		t.Fatalf("source = %q", source)
	}
	want := uint32(uint64(50) * samplesPerFrame * 1000 / 44100)
	if ms != want {
		t.Fatalf("duration = %d, want %d", ms, want)
	}
}

func TestXingDuration(t *testing.T) {
	const header = 0xfffba004
	fh, _ := parseHeader(header)
	frameSize := fh.length
	first := mpeg1Frame(header, frameSize)
	off := 4 + 17
	copy(first[off:off+4], "Xing")
	binary.BigEndian.PutUint32(first[off+4:off+8], 0x1)  // frames-present flag
	binary.BigEndian.PutUint32(first[off+8:off+12], 100) // 100 frames
	var audio []byte
	audio = append(audio, first...)
	for i := 1; i < 100; i++ {
		audio = append(audio, mpeg1Frame(header, frameSize)...)
	}
	ms, source := DurationMillis(audio, 0)
	if source != "xing" {
		t.Fatalf("source = %q", source)
	}
	want := uint32(uint64(100) * 1152 * 1000 / 44100)
	if ms != want {
		t.Fatalf("xing duration = %d, want %d", ms, want)
	}
}

func TestFallsBackToTLEN(t *testing.T) {
	ms, source := DurationMillis([]byte("not audio at all"), 12345)
	if source != "tlen" || ms != 12345 {
		t.Fatalf("tlen fallback: %d from %q", ms, source)
	}
}
