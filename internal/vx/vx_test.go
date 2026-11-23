package vx

import (
	"bytes"
	"testing"
)

func TestRoundTrip(t *testing.T) {
	header := Header{
		Version: 1, Title: "t", Width: 4, Height: 2,
		RateNum: 24, RateDen: 1, Format: "yuv444p8",
		Color: Color{Primaries: "bt709", Transfer: "bt1886", Matrix: "bt709", Range: "limited"},
		Audio: Audio{SampleRate: 48000, Channels: 1, Depth: 16},
	}
	var buf bytes.Buffer
	vw, err := NewWriter(&buf, header)
	if err != nil {
		t.Fatal(err)
	}
	plane := header.Width * header.Height
	frame := Frame{Num: 3, PTS: 3, Key: true, Cut: true,
		Y: make([]byte, plane), U: make([]byte, plane), V: make([]byte, plane)}
	frame.Y[0] = 200
	if err := vw.WriteFrame(frame); err != nil {
		t.Fatal(err)
	}
	if err := vw.WriteAudio(AudioPacket{SampleStart: 10, Samples: []int16{1, -2, 300}}); err != nil {
		t.Fatal(err)
	}
	if err := vw.WriteSubtitle(Subtitle{ID: 1, Start: 0, End: 9, Text: "hi"}); err != nil {
		t.Fatal(err)
	}
	if err := vw.Close(); err != nil {
		t.Fatal(err)
	}

	got, err := Read(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Frames) != 1 || got.Frames[0].Num != 3 || !got.Frames[0].Cut {
		t.Fatalf("frame round trip failed: %+v", got.Frames)
	}
	if got.Frames[0].Y[0] != 200 {
		t.Fatalf("plane data lost")
	}
	if len(got.Audio) != 1 || got.Audio[0].SampleStart != 10 || got.Audio[0].Samples[1] != -2 {
		t.Fatalf("audio round trip failed: %+v", got.Audio)
	}
	if len(got.Subtitles) != 1 || got.Subtitles[0].Text != "hi" {
		t.Fatalf("subtitle round trip failed: %+v", got.Subtitles)
	}
}

func TestBadMagic(t *testing.T) {
	if _, err := Read(bytes.NewReader([]byte("XXXX"))); err == nil {
		t.Fatal("expected magic error")
	}
}
