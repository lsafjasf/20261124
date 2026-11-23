package au

import (
	"bytes"
	"encoding/binary"
	"errors"
	"testing"
)

func makeAU(offset, size, encoding, rate, channels uint32, payload []byte) []byte {
	n := MinHeaderSize
	if int(offset) > n {
		n = int(offset)
	}
	n += len(payload)
	b := make([]byte, n)
	copy(b, Magic)
	binary.BigEndian.PutUint32(b[4:8], offset)
	binary.BigEndian.PutUint32(b[8:12], size)
	binary.BigEndian.PutUint32(b[12:16], encoding)
	binary.BigEndian.PutUint32(b[16:20], rate)
	binary.BigEndian.PutUint32(b[20:24], channels)
	if int(offset) <= n && len(payload) > 0 {
		copy(b[int(offset):], payload)
	}
	return b
}

func TestParseUsesBigEndianHeader(t *testing.T) {
	payload := []byte{0x11, 0x22, 0x33, 0x44}
	raw := makeAU(32, uint32(len(payload)), uint32(EncodingLinear16), 48000, 2, payload)
	before := append([]byte(nil), raw...)

	f, err := Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	if f.Header.DataOffset != 32 || f.Header.DataSize != 4 ||
		f.Header.Encoding != EncodingLinear16 || f.Header.SampleRate != 48000 ||
		f.Header.Channels != 2 {
		t.Fatalf("header = %+v", f.Header)
	}
	if f.Format.Kind != KindLinearPCM || f.Format.BitsPerSample != 16 ||
		f.Format.BytesPerSample != 2 || f.Format.BlockAlign != 4 {
		t.Fatalf("format = %+v", f.Format)
	}
	if f.AvailableBytes != 4 || f.DeclaredBytes != 4 || f.Frames != 1 ||
		f.AvailableFrames != 1 || f.DeclaredFrames != 1 {
		t.Fatalf("counts = %+v", f)
	}
	if f.DeclaredSampleCount() != 2 || f.AvailableSampleCount() != 2 || f.SampleCount() != 2 {
		t.Fatalf("samples = declared %d available %d kept %d",
			f.DeclaredSampleCount(), f.AvailableSampleCount(), f.SampleCount())
	}
	if !bytes.Equal(f.Data, payload) {
		t.Fatalf("data = %x, want %x", f.Data, payload)
	}
	if &f.Data[0] != &raw[32] {
		t.Fatal("payload was copied instead of aliasing the input")
	}
	if !bytes.Equal(raw, before) {
		t.Fatal("Parse rewrote input bytes")
	}
}

func TestEncodingClassification(t *testing.T) {
	tests := []struct {
		encoding Encoding
		kind     Kind
		bits     uint16
		bytes    uint16
	}{
		{EncodingMuLaw, KindMuLaw, 8, 1},
		{EncodingLinear8, KindLinearPCM, 8, 1},
		{EncodingLinear16, KindLinearPCM, 16, 2},
		{EncodingLinear24, KindLinearPCM, 24, 3},
		{EncodingLinear32, KindLinearPCM, 32, 4},
		{EncodingFloat32, KindUnknown, 0, 0},
		{EncodingFloat64, KindUnknown, 0, 0},
		{EncodingALaw, KindUnknown, 0, 0},
		{Encoding(99), KindUnknown, 0, 0},
	}
	for _, tt := range tests {
		t.Run(tt.encoding.String(), func(t *testing.T) {
			payload := []byte{1, 2, 3, 4, 5, 6}
			f, err := Parse(makeAU(24, 6, uint32(tt.encoding), 8000, 1, payload))
			if err != nil {
				t.Fatal(err)
			}
			if f.Format.Kind != tt.kind || f.Format.BitsPerSample != tt.bits ||
				f.Format.BytesPerSample != tt.bytes {
				t.Fatalf("format = %+v", f.Format)
			}
			if tt.kind == KindUnknown {
				if !bytes.Equal(f.Data, payload) {
					t.Fatalf("unknown payload = %x, want %x", f.Data, payload)
				}
				if _, err := f.Samples(); !errors.Is(err, ErrUnknownEncoding) {
					t.Fatalf("Samples error = %v, want ErrUnknownEncoding", err)
				}
			}
		})
	}
}

func TestTruncatedPayloadKeepsCompleteFrames(t *testing.T) {
	payload := []byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06}
	raw := makeAU(24, 20, uint32(EncodingLinear16), 44100, 2, payload)
	before := append([]byte(nil), raw...)

	f, err := Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	if !f.Truncated || !f.SizeMismatch {
		t.Fatalf("truncated=%v sizeMismatch=%v", f.Truncated, f.SizeMismatch)
	}
	if f.DeclaredBytes != 20 || f.AvailableBytes != 6 {
		t.Fatalf("bytes = declared %d available %d", f.DeclaredBytes, f.AvailableBytes)
	}
	if f.DeclaredFrames != 5 || f.AvailableFrames != 1 || f.Frames != 1 {
		t.Fatalf("frames = declared %d available %d kept %d",
			f.DeclaredFrames, f.AvailableFrames, f.Frames)
	}
	if !bytes.Equal(f.Data, payload[:4]) {
		t.Fatalf("data = %x, want %x", f.Data, payload[:4])
	}
	if !bytes.Equal(f.ExtraBytes, payload[4:]) {
		t.Fatalf("extra = %x, want %x", f.ExtraBytes, payload[4:])
	}
	if f.PartialBytes != 2 {
		t.Fatalf("partial bytes = %d, want 2", f.PartialBytes)
	}
	if !f.IncompleteTail {
		t.Fatal("IncompleteTail = false, want true")
	}
	if !bytes.Equal(raw, before) {
		t.Fatal("Parse rewrote input bytes")
	}
}

func TestDeclaredSizeSmallerThanPayload(t *testing.T) {
	payload := []byte{0x00, 0x01, 0x00, 0x02, 0xaa, 0xbb}
	f, err := Parse(makeAU(24, 4, uint32(EncodingLinear16), 8000, 1, payload))
	if err != nil {
		t.Fatal(err)
	}
	if f.Truncated || !f.SizeMismatch {
		t.Fatalf("truncated=%v sizeMismatch=%v", f.Truncated, f.SizeMismatch)
	}
	if f.IncompleteTail || f.PartialBytes != 0 {
		t.Fatalf("incompleteTail=%v partial=%d", f.IncompleteTail, f.PartialBytes)
	}
	if f.Frames != 2 || !bytes.Equal(f.Data, payload[:4]) {
		t.Fatalf("frames=%d data=%x", f.Frames, f.Data)
	}
	if !bytes.Equal(f.ExtraBytes, payload[4:]) {
		t.Fatalf("extra = %x, want %x", f.ExtraBytes, payload[4:])
	}
}

func TestUnknownDataSizeUsesRemainingBytes(t *testing.T) {
	payload := []byte{0x00, 0x01, 0x00, 0x02, 0x00, 0x03}
	f, err := Parse(makeAU(24, UnknownDataSize, uint32(EncodingLinear16), 8000, 1, payload))
	if err != nil {
		t.Fatal(err)
	}
	if f.SizeMismatch || f.Truncated || f.DeclaredBytes != 6 || f.Frames != 3 {
		t.Fatalf("file = %+v", f)
	}
	if !bytes.Equal(f.Data, payload) {
		t.Fatalf("data = %x, want %x", f.Data, payload)
	}
}

func TestOffsetAtOrBeyondEOFIsSafe(t *testing.T) {
	t.Run("at EOF", func(t *testing.T) {
		f, err := Parse(makeAU(24, 0, uint32(EncodingMuLaw), 8000, 1, nil))
		if err != nil {
			t.Fatal(err)
		}
		if f.AvailableBytes != 0 || f.Frames != 0 || f.Truncated {
			t.Fatalf("file = %+v", f)
		}
	})
	t.Run("beyond EOF", func(t *testing.T) {
		raw := makeAU(24, 10, uint32(EncodingMuLaw), 8000, 1, nil)
		binary.BigEndian.PutUint32(raw[4:8], 100)
		f, err := Parse(raw)
		if err != nil {
			t.Fatal(err)
		}
		if f.AvailableBytes != 0 || f.Frames != 0 || !f.Truncated || !f.SizeMismatch {
			t.Fatalf("file = %+v", f)
		}
		if !f.OffsetPastEOF {
			t.Fatal("OffsetPastEOF = false, want true")
		}
		if f.Data != nil || f.ExtraBytes != nil {
			t.Fatalf("data=%v extra=%v", f.Data, f.ExtraBytes)
		}
	})
}

func TestHeaderErrors(t *testing.T) {
	valid := makeAU(24, 0, uint32(EncodingMuLaw), 8000, 1, nil)
	tests := []struct {
		name string
		data []byte
		want error
	}{
		{"short", valid[:20], ErrShortHeader},
		{"magic", append([]byte("nope"), valid[4:]...), ErrBadMagic},
		{"offset", makeAU(20, 0, uint32(EncodingMuLaw), 8000, 1, nil), ErrInvalidOffset},
		{"channels", makeAU(24, 0, uint32(EncodingMuLaw), 8000, 0, nil), ErrInvalidChannels},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := Parse(tt.data); !errors.Is(err, tt.want) {
				t.Fatalf("error = %v, want %v", err, tt.want)
			}
		})
	}
}
