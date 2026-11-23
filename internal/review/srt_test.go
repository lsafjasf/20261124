package review

import (
	"strings"
	"testing"
)

func TestParseSRTWithMultilineAndCRLF(t *testing.T) {
	input := "\ufeff1\r\n00:00:01,250 --> 00:00:03,500\r\n第一行\r\n第二行\r\n\r\n2\r\n00:00:04.000 --> 00:00:05.100\r\nHello!\r\n"
	cues, err := ParseSRT(strings.NewReader(input))
	if err != nil {
		t.Fatal(err)
	}
	if len(cues) != 2 {
		t.Fatalf("got %d cues, want 2", len(cues))
	}
	if cues[0].ID != "1" || cues[0].StartMs != 1250 || cues[0].EndMs != 3500 {
		t.Fatalf("unexpected first cue: %+v", cues[0])
	}
	if cues[0].Text != "第一行\n第二行" {
		t.Fatalf("unexpected text: %q", cues[0].Text)
	}
	if cues[1].StartMs != 4000 || cues[1].EndMs != 5100 {
		t.Fatalf("unexpected second cue: %+v", cues[1])
	}
}

func TestParseSRTRejectsInvalidTiming(t *testing.T) {
	input := "1\n00:00:03,000 --> 00:00:02,000\nbad\n"
	if _, err := ParseSRT(strings.NewReader(input)); err == nil {
		t.Fatal("expected invalid timing error")
	}
}

func TestParseJSONCuesAcceptsArray(t *testing.T) {
	input := `[{"id":"a","startMs":100,"endMs":200,"text":"x","region":{"x":0,"y":0,"width":1,"height":1}}]`
	cues, err := ParseJSONCues(strings.NewReader(input))
	if err != nil {
		t.Fatal(err)
	}
	if len(cues) != 1 || cues[0].ID != "a" || cues[0].Region == nil {
		t.Fatalf("unexpected cues: %+v", cues)
	}
}
