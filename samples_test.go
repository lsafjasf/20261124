package au

import (
	"bytes"
	"reflect"
	"testing"
)

func TestSamplesLinearPCM(t *testing.T) {
	tests := []struct {
		name     string
		encoding Encoding
		channels uint32
		payload  []byte
		want     []int32
	}{
		{
			name:     "8-bit signed",
			encoding: EncodingLinear8,
			channels: 1,
			payload:  []byte{0x7f, 0xff, 0x80, 0x00},
			want:     []int32{127, -1, -128, 0},
		},
		{
			name:     "16-bit big endian stereo",
			encoding: EncodingLinear16,
			channels: 2,
			payload:  []byte{0x00, 0x01, 0xff, 0xfe, 0x80, 0x00, 0x7f, 0xff},
			want:     []int32{1, -2, -32768, 32767},
		},
		{
			name:     "24-bit sign extension",
			encoding: EncodingLinear24,
			channels: 1,
			payload: []byte{
				0x00, 0x00, 0x01,
				0xff, 0xff, 0xff,
				0x80, 0x00, 0x00,
				0x7f, 0xff, 0xff,
			},
			want: []int32{1, -1, -8388608, 8388607},
		},
		{
			name:     "32-bit big endian",
			encoding: EncodingLinear32,
			channels: 1,
			payload:  []byte{0x7f, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff},
			want:     []int32{2147483647, -1},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			raw := makeAU(24, uint32(len(tt.payload)),
				uint32(tt.encoding), 8000, tt.channels, tt.payload)
			f, err := Parse(raw)
			if err != nil {
				t.Fatal(err)
			}
			got, err := f.Samples()
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("samples = %v, want %v", got, tt.want)
			}
			if !bytes.Equal(f.Data, tt.payload) {
				t.Fatal("sample decoding changed payload bytes")
			}
		})
	}
}

func TestSamplesMuLaw(t *testing.T) {
	tests := []struct {
		in   byte
		want int16
	}{
		{0xff, 0},
		{0x7f, 0},
		{0x00, -32124},
		{0x80, 32124},
	}
	for _, tt := range tests {
		if got := DecodeMuLaw(tt.in); got != tt.want {
			t.Errorf("DecodeMuLaw(%#02x) = %d, want %d", tt.in, got, tt.want)
		}
	}

	payload := []byte{0xff, 0x7f, 0x00, 0x80}
	f, err := Parse(makeAU(24, 4, uint32(EncodingMuLaw), 8000, 1, payload))
	if err != nil {
		t.Fatal(err)
	}
	got, err := f.Samples()
	if err != nil {
		t.Fatal(err)
	}
	want := []int32{0, 0, -32124, 32124}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("samples = %v, want %v", got, want)
	}
}

func TestSamplesIgnoreIncompleteTrailingByte(t *testing.T) {
	payload := []byte{0x00, 0x01, 0x00, 0x02, 0x7f}
	f, err := Parse(makeAU(24, 5, uint32(EncodingLinear16), 8000, 1, payload))
	if err != nil {
		t.Fatal(err)
	}
	got, err := f.Samples()
	if err != nil {
		t.Fatal(err)
	}
	want := []int32{1, 2}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("samples = %v, want %v", got, want)
	}
	if !bytes.Equal(f.ExtraBytes, []byte{0x7f}) {
		t.Fatalf("extra = %x", f.ExtraBytes)
	}
}
