package bitstream

import (
	"testing"

	"hdr10audit/hdr"
)

func TestScanHEVCDirectSEI(t *testing.T) {
	m := hdr.Mastering{
		Primaries: hdr.Primaries{
			{X: 13250, Y: 34500}, {X: 7500, Y: 3000}, {X: 34000, Y: 16000},
		},
		WhitePoint:   hdr.Point{X: 15635, Y: 16450},
		MaxLuminance: 10000000, MinLuminance: 1,
	}
	c := hdr.ContentLight{MaxCLL: 1000, MaxFALL: 400}
	rbsp := []byte{137, 24}
	rbsp = append(rbsp, hdr.EncodeMastering(m)...)
	rbsp = append(rbsp, 144, 4)
	rbsp = append(rbsp, hdr.EncodeContentLight(c)...)
	rbsp = append(rbsp, 0x80)
	nal := append([]byte{0x4e, 0x01}, escapeForTest(rbsp)...)
	got := ScanNAL(nal, HEVC, "test")
	if len(got.Issues) != 0 {
		t.Fatalf("unexpected issues: %+v", got.Issues)
	}
	if len(got.Occurrences) != 2 {
		t.Fatalf("occurrences=%d, want 2", len(got.Occurrences))
	}
	if *got.Occurrences[0].Metadata.Mastering != m {
		t.Fatalf("mastering mismatch: %+v", got.Occurrences[0].Metadata.Mastering)
	}
	if *got.Occurrences[1].Metadata.Light != c {
		t.Fatalf("light mismatch: %+v", got.Occurrences[1].Metadata.Light)
	}
}

func TestScanAVCT35HDR10(t *testing.T) {
	m, c := hdr.Mastering{
		Primaries: hdr.Primaries{
			{X: 13250, Y: 34500}, {X: 7500, Y: 3000}, {X: 34000, Y: 16000},
		},
		WhitePoint:   hdr.Point{X: 15635, Y: 16450},
		MaxLuminance: 10000000, MinLuminance: 1,
	}, hdr.ContentLight{MaxCLL: 1000, MaxFALL: 400}
	payload := []byte{0xb5, 0x00, 0x3c, 0x00, 0x01, 0x04, 0x00}
	payload = append(payload, hdr.EncodeMastering(m)...)
	payload = append(payload, hdr.EncodeContentLight(c)...)
	rbsp := []byte{4, byte(len(payload))}
	rbsp = append(rbsp, payload...)
	rbsp = append(rbsp, 0x80)
	nal := append([]byte{0x06}, rbsp...)
	got := ScanNAL(nal, AVC, "test")
	if len(got.Issues) != 0 || len(got.Occurrences) != 1 {
		t.Fatalf("issues=%+v occurrences=%+v", got.Issues, got.Occurrences)
	}
	md := got.Occurrences[0].Metadata
	if md.Mastering == nil || md.Light == nil {
		t.Fatalf("incomplete T.35 metadata: %+v", md)
	}
	if *md.Mastering != m || *md.Light != c {
		t.Fatalf("T.35 values mismatch: %+v", md)
	}
}

func TestUnescapeRBSP(t *testing.T) {
	in := []byte{0x00, 0x00, 0x03, 0x00, 0x00, 0x03, 0x01}
	want := []byte{0x00, 0x00, 0x00, 0x00, 0x01}
	got := UnescapeRBSP(in)
	if string(got) != string(want) {
		t.Fatalf("got %x want %x", got, want)
	}
}

func escapeForTest(data []byte) []byte {
	out := make([]byte, 0, len(data))
	zeros := 0
	for _, b := range data {
		if zeros >= 2 && b <= 3 {
			out = append(out, 3)
			zeros = 0
		}
		out = append(out, b)
		if b == 0 {
			zeros++
		} else {
			zeros = 0
		}
	}
	return out
}
