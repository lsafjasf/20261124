package main

import "testing"

// stereoBurst builds an interleaved-stereo burst: duplicated preamble,
// payload of the given word count, then zero stuffing.
func stereoBurst(dataType uint8, burstMode uint8, lengthBits uint16, payloadWords, stuffWords int) []uint16 {
	pc := uint16(dataType) | uint16(burstMode)<<13
	out := []uint16{syncPa, syncPa, syncPb, syncPb, pc, pc, lengthBits, lengthBits}
	for k := 0; k < payloadWords; k++ {
		out = append(out, uint16(0x1000+k))
	}
	for k := 0; k < stuffWords; k++ {
		out = append(out, 0)
	}
	return out
}

func pcmRun(n int) []uint16 {
	out := make([]uint16, n)
	for i := range out {
		out[i] = uint16(0x0100 + i%0x40)
	}
	return out
}

func TestPCMOnlyNotMisreported(t *testing.T) {
	segs := Parse(pcmRun(500))
	if len(segs) != 1 || segs[0].Kind != SegPCM {
		t.Fatalf("expected single PCM segment, got %+v", segs)
	}
	if segs[0].StartWord != 0 || segs[0].EndWord != 500 {
		t.Fatalf("bad PCM range: %+v", segs[0])
	}
}

func TestStereoBurstRoundTrip(t *testing.T) {
	var stream []uint16
	stream = append(stream, pcmRun(64)...)
	stream = append(stream, stereoBurst(0x01, 0, 256, 16, 8)...)
	stream = append(stream, pcmRun(32)...)

	segs := Parse(stream)
	if len(segs) != 3 {
		t.Fatalf("expected 3 segments, got %d: %+v", len(segs), segs)
	}
	b := segs[1]
	if b.Kind != SegBurst || b.Pc.DataType != 0x01 || b.TypeName != "AC-3" {
		t.Fatalf("bad burst: %+v", b)
	}
	if b.StartWord != 64 || b.EndWord != 64+8+16+8 {
		t.Fatalf("bad burst range: %+v", b)
	}
	if b.LengthBits != 256 || b.PayloadWords != 16 || b.PadBits != 0 || b.StuffWords != 8 {
		t.Fatalf("bad burst lengths: %+v", b)
	}
	if len(b.Errors) != 0 {
		t.Fatalf("unexpected errors: %v", b.Errors)
	}
}

func TestOddLengthPadding(t *testing.T) {
	segs := Parse(stereoBurst(0x0B, 0, 20, 2, 0))
	if len(segs) != 1 || segs[0].PayloadWords != 2 || segs[0].PadBits != 12 {
		t.Fatalf("bad padding: %+v", segs)
	}
}

func TestUnknownTypeKeepsRawPayload(t *testing.T) {
	segs := Parse(stereoBurst(0x1B, 2, 64, 4, 0))
	if len(segs) != 1 || segs[0].Kind != SegBurst {
		t.Fatalf("expected one burst, got %+v", segs)
	}
	b := segs[0]
	if b.KnownType || b.TypeName != "Unknown(0x1B)" || b.Pc.BurstMode != 2 {
		t.Fatalf("bad unknown-type burst: %+v", b)
	}
	want := []uint16{0x1000, 0x1001, 0x1002, 0x1003}
	for i, w := range want {
		if b.Payload[i] != w {
			t.Fatalf("payload word %d = %#04x, want %#04x", i, b.Payload[i], w)
		}
	}
}

func TestTruncatedAtEndOfStream(t *testing.T) {
	stream := stereoBurst(0x01, 0, 256, 6, 0)
	segs := Parse(stream)
	if len(segs) != 1 || len(segs[0].Errors) == 0 {
		t.Fatalf("expected truncation error, got %+v", segs)
	}
	if segs[0].EndWord != len(stream) {
		t.Fatalf("truncated burst should extend to end of stream: %+v", segs[0])
	}
}

func TestTruncatedByNextPreamble(t *testing.T) {
	var stream []uint16
	stream = append(stream, stereoBurst(0x01, 0, 512, 8, 0)...)
	stream = append(stream, stereoBurst(0x01, 0, 16, 1, 0)...)
	segs := Parse(stream)
	if len(segs) != 2 || segs[0].Kind != SegBurst || segs[1].Kind != SegBurst {
		t.Fatalf("expected two bursts, got %+v", segs)
	}
	if len(segs[0].Errors) == 0 {
		t.Fatalf("expected truncation-by-preamble error on first burst")
	}
}

func TestIllegalSyncWord(t *testing.T) {
	stream := pcmRun(16)
	stream = append(stream, syncPa, 0x1234)
	stream = append(stream, pcmRun(16)...)
	segs := Parse(stream)
	if len(segs) != 3 || segs[1].Kind != SegError {
		t.Fatalf("expected PCM/ERROR/PCM, got %+v", segs)
	}
	if segs[1].StartWord != 16 || segs[1].EndWord != 17 {
		t.Fatalf("bad error range: %+v", segs[1])
	}
}

func TestByteOrderError(t *testing.T) {
	stream := pcmRun(8)
	stream = append(stream, syncPaSwapped, syncPaSwapped, syncPbSwapped, syncPbSwapped, 0, 0, 0, 0)
	stream = append(stream, pcmRun(8)...)
	segs := Parse(stream)
	if len(segs) != 3 || segs[1].Kind != SegError {
		t.Fatalf("expected byte-order error segment, got %+v", segs)
	}
	if segs[1].EndWord-segs[1].StartWord != 8 {
		t.Fatalf("bad byte-order segment: %+v", segs[1])
	}
}

func TestMonoLayout(t *testing.T) {
	stream := []uint16{syncPa, syncPb, 0x01, 32, 0xAAAA, 0xBBBB}
	segs := Parse(stream)
	if len(segs) != 1 || segs[0].Kind != SegBurst || segs[0].PayloadWords != 2 {
		t.Fatalf("bad mono burst: %+v", segs)
	}
	if segs[0].Payload[0] != 0xAAAA || segs[0].Payload[1] != 0xBBBB {
		t.Fatalf("bad mono payload: %+v", segs[0].Payload)
	}
}
