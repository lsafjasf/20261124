package subtitle

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestParseProjectSample(t *testing.T) {
	p, err := LoadProject(filepath.Join("..", "testdata", "reel_500s.json"))
	if err != nil {
		t.Fatal(err)
	}
	if p.VideoRate.String() != "24000/1001" {
		t.Fatalf("video rate = %s", p.VideoRate)
	}
	if p.NominalRate.String() != "24" {
		t.Fatalf("nominal rate = %s", p.NominalRate)
	}
	if !p.UseCentiseconds() {
		t.Fatal("sample should enable ASS centiseconds")
	}
	if p.DurationFrames() != 11988 {
		t.Fatalf("duration = %d frames", p.DurationFrames())
	}
}

func TestTimestampRoundTrip(t *testing.T) {
	for _, ms := range []int64{0, 1, 1001, 499999, 500000, 3_600_000, 3_661_002} {
		text := FormatTimestamp(ms)
		got, err := ParseTimestamp(text)
		if err != nil {
			t.Fatalf("ParseTimestamp(%q): %v", text, err)
		}
		if got != ms {
			t.Fatalf("round trip %d -> %q -> %d", ms, text, got)
		}
	}
}

func TestSRTRoundTrip(t *testing.T) {
	text := "1\n00:00:01,000 --> 00:00:02,500\n第一行\n第二行\n\n" +
		"2\n00:08:19,500 --> 00:08:20,000\n片尾\n\n"
	cues, err := ParseSRT(strings.NewReader(text))
	if err != nil {
		t.Fatal(err)
	}
	if len(cues) != 2 {
		t.Fatalf("parsed %d cues, want 2", len(cues))
	}
	if cues[0].StartMS != 1000 || cues[0].EndMS != 2500 || cues[0].Text != "第一行\n第二行" {
		t.Fatalf("unexpected first cue: %+v", cues[0])
	}
	if got := FormatSRT(cues); got != text {
		t.Fatalf("SRT round trip mismatch:\n%s\nwant:\n%s", got, text)
	}
}

func TestParseTimestampMillisecondWidths(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want int64
	}{
		{"00:00:01,5", 1500},
		{"00:00:01,05", 1050},
		{"00:00:01,005", 1005},
	} {
		got, err := ParseTimestamp(tc.in)
		if err != nil {
			t.Fatalf("ParseTimestamp(%q): %v", tc.in, err)
		}
		if got != tc.want {
			t.Fatalf("ParseTimestamp(%q) = %d, want %d", tc.in, got, tc.want)
		}
	}
}
