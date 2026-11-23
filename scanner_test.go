package truehd

import (
	"bytes"
	"encoding/hex"
	"testing"
)

// This is a valid 48 kHz mono TrueHD major-sync header produced by the
// ffmpeg TrueHD encoder. It makes the synthetic fixtures exercise the real
// checksum representation rather than a self-consistent reimplementation.
const fixtureMajorHex = "f8726fba00f16002b752000000008c7f10140000000000000000691a"

type fixture struct {
	data          []byte
	starts        []int
	ends          []int
	payloadStarts []int
	major         []bool
}

func buildFixture(t *testing.T, frames int) *fixture {
	t.Helper()
	f := &fixture{}
	for i := 0; i < frames; i++ {
		major := i%16 == 0
		payloadLen := 32
		if major {
			payloadLen = 64
		}
		packet := buildPacket(t, major, payloadLen, byte(i))
		f.starts = append(f.starts, len(f.data))
		f.ends = append(f.ends, len(f.data)+len(packet))
		f.payloadStarts = append(f.payloadStarts, len(f.data)+packetPayloadOffset(major))
		f.major = append(f.major, major)
		f.data = append(f.data, packet...)
	}
	return f
}

func packetPayloadOffset(major bool) int {
	if major {
		return 4 + majorHeaderBase + 2
	}
	return 4 + 2
}

func buildPacket(t *testing.T, major bool, payloadLen int, seed byte) []byte {
	t.Helper()
	majorHeader, err := hex.DecodeString(fixtureMajorHex)
	if err != nil {
		t.Fatal(err)
	}
	headerLen := 4
	if major {
		headerLen += len(majorHeader)
	}
	auLen := headerLen + 2 + payloadLen
	if auLen > 0x1ffe {
		t.Fatalf("fixture packet too large: %d", auLen)
	}

	packet := make([]byte, auLen)
	packet[0] = byte((auLen / 2) >> 8)
	packet[1] = byte(auLen / 2)
	if major {
		copy(packet[4:], majorHeader)
	}
	flags := byte(0x60) // non-restart substream
	if major {
		flags = 0x20 // restart substream
	}
	packet[headerLen] = flags | byte((payloadLen/2)>>8)&0x0f
	packet[headerLen+1] = byte(payloadLen / 2)

	payload := packet[headerLen+2:]
	for i := 0; i < len(payload); i++ {
		payload[i] = seed + byte(i*7)
	}
	n := len(payload)
	payload[n-2] = xorBytes(payload[:n-2]) ^ 0xa9
	payload[n-1] = crc8MLP(payload[:n-3], 0x3c) ^ payload[n-3]

	p := xorBytes(packet[0:3])
	p ^= xorBytes(packet[headerLen : headerLen+2])
	packet[3] = ((p >> 4) ^ p ^ 0x0f) & 0x0f
	return packet
}

func TestSubstreamCheckKnownFixture(t *testing.T) {
	f := buildFixture(t, 1)
	cfg, failure := parseMajorSync(f.data, 4)
	if failure != nil {
		t.Fatalf("major sync rejected: %v", failure)
	}
	if cfg.NumSubstreams != 1 || cfg.SampleRate != 48000 || cfg.SamplesPerFrame != 40 {
		t.Fatalf("unexpected config: %+v", cfg)
	}
	fh, failure := inspectFrame(f.data, 0, nil)
	if failure != nil {
		t.Fatalf("frame rejected: %v", failure)
	}
	if _, checked, ok := validatePayload(f.data, 0, fh); !ok || !checked {
		t.Fatalf("payload check: checked=%t ok=%t", checked, ok)
	}
}

func TestIntactFixture(t *testing.T) {
	f := buildFixture(t, 40)
	r := Scan(f.data)
	if !r.Complete || r.Truncated || len(r.Issues) != 0 {
		t.Fatalf("unexpected report: %+v", r)
	}
	if len(r.Frames) != 40 || r.PlayableBytes != int64(len(f.data)) {
		t.Fatalf("frames=%d playable=%d want 40/%d", len(r.Frames), r.PlayableBytes, len(f.data))
	}
	if r.PlayableSamples != 40*40 || r.UncheckedPayloadFrames != 0 {
		t.Fatalf("samples=%d unchecked=%d", r.PlayableSamples, r.UncheckedPayloadFrames)
	}
	if len(r.PlayableRanges) != 1 || r.PlayableRanges[0].RawEnd != int64(len(f.data)) {
		t.Fatalf("playable ranges: %+v", r.PlayableRanges)
	}
}

func TestTruncatedTailKeepsCompleteFrames(t *testing.T) {
	f := buildFixture(t, 20)
	cut := 3
	data := append([]byte(nil), f.data[:f.ends[19]-cut]...)
	r := Scan(data)

	if r.HeaderErrors != 0 || r.PayloadErrors != 0 || r.EOFErrors != 1 || !r.Truncated {
		t.Fatalf("counts header=%d payload=%d eof=%d truncated=%t",
			r.HeaderErrors, r.PayloadErrors, r.EOFErrors, r.Truncated)
	}
	if len(r.Frames) != 19 {
		t.Fatalf("kept %d frames, want 19", len(r.Frames))
	}
	if got := r.Frames[len(r.Frames)-1].RawEnd; got != int64(f.starts[19]) {
		t.Fatalf("last playable byte=%d, want %d", got, f.starts[19])
	}
	issue := r.Issues[0]
	if issue.Kind != EOFError || issue.RawStart != int64(f.starts[19]) || issue.RawEnd != int64(len(data)) {
		t.Fatalf("eof issue: %+v", issue)
	}
	if issue.DeclaredSize != f.ends[19]-f.starts[19] {
		t.Fatalf("declared=%d, want %d", issue.DeclaredSize, f.ends[19]-f.starts[19])
	}
}

func TestHeaderPayloadAndEOFAreSeparate(t *testing.T) {
	f := buildFixture(t, 40)
	data := append([]byte(nil), f.data...)

	// Break packet framing at frame 3. The scanner must resync at frame 4.
	data[f.starts[3]] = 0xff
	data[f.starts[3]+1] = 0xff

	// A payload bit flip leaves the access-unit framing intact.
	data[f.payloadStarts[10]+5] ^= 0x40

	// Cut inside the final packet.
	data = data[:f.ends[39]-2]
	r := Scan(data)

	if r.HeaderErrors != 1 || r.PayloadErrors != 1 || r.EOFErrors != 1 {
		t.Fatalf("counts header=%d payload=%d eof=%d", r.HeaderErrors, r.PayloadErrors, r.EOFErrors)
	}
	if len(r.Frames) != 37 {
		t.Fatalf("kept %d frames, want 37", len(r.Frames))
	}
	if len(r.PlayableRanges) != 3 {
		t.Fatalf("playable ranges=%d, want 3", len(r.PlayableRanges))
	}
	if len(r.DiscardedPackets) != 3 {
		t.Fatalf("discarded packets=%d, want 3", len(r.DiscardedPackets))
	}

	byKind := map[ErrorKind]Issue{}
	for _, issue := range r.Issues {
		byKind[issue.Kind] = issue
	}
	if got := byKind[HeaderError]; got.RawStart != int64(f.starts[3]) || got.RawEnd != int64(f.starts[4]) {
		t.Fatalf("header range: %+v", got)
	}
	if got := byKind[PayloadError]; got.RawStart != int64(f.starts[10]) || got.RawEnd != int64(f.ends[10]) {
		t.Fatalf("payload range: %+v", got)
	}
	if got := byKind[EOFError]; got.RawStart != int64(f.starts[39]) || got.RawEnd != int64(len(data)) {
		t.Fatalf("eof range: %+v", got)
	}
}

func TestResyncToNextMajorSync(t *testing.T) {
	f := buildFixture(t, 40)
	data := append([]byte(nil), f.data...)
	for i := f.starts[2]; i < f.starts[16]; i++ {
		data[i] = 0xff
	}
	r := Scan(data)

	if r.HeaderErrors != 1 || r.PayloadErrors != 0 || r.EOFErrors != 0 {
		t.Fatalf("counts header=%d payload=%d eof=%d", r.HeaderErrors, r.PayloadErrors, r.EOFErrors)
	}
	if len(r.Frames) != 26 {
		t.Fatalf("kept %d frames, want 26 (0,1 and 16..39)", len(r.Frames))
	}
	if r.Frames[2].RawStart != int64(f.starts[16]) || !r.Frames[2].MajorSync {
		t.Fatalf("did not resume at major sync: %+v", r.Frames[2])
	}
	if r.Issues[0].RawStart != int64(f.starts[2]) || r.Issues[0].RawEnd != int64(f.starts[16]) {
		t.Fatalf("discard range: %+v", r.Issues[0])
	}
}

func TestNoMajorSyncIsHeaderError(t *testing.T) {
	data := bytes.Repeat([]byte{0x42}, 200)
	r := Scan(data)
	if r.HeaderErrors != 1 || r.EOFErrors != 0 || len(r.Frames) != 0 {
		t.Fatalf("unexpected report: %+v", r)
	}
	if r.Issues[0].RawStart != 0 || r.Issues[0].RawEnd != 200 {
		t.Fatalf("issue range: %+v", r.Issues[0])
	}
}

func TestTruncatedAfterAccessUnitHeaderIsEOF(t *testing.T) {
	f := buildFixture(t, 1)
	r := Scan(f.data[:4])
	if r.EOFErrors != 1 || r.HeaderErrors != 0 || len(r.Frames) != 0 {
		t.Fatalf("unexpected report: %+v", r)
	}
	if r.Issues[0].RawStart != 0 || r.Issues[0].RawEnd != 4 {
		t.Fatalf("issue range: %+v", r.Issues[0])
	}
}
