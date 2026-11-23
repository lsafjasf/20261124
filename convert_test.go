package main

import (
	"errors"
	"strings"
	"testing"
)

func TestConvertBasicStyleAndLineBreaks(t *testing.T) {
	input := "\xef\xbb\xbf{0}{25}{Y:i}Hello|world\r\n"
	result, err := Convert([]byte(input), 25)
	if err != nil {
		t.Fatalf("Convert returned error: %v", err)
	}
	if result.CueCount != 1 || len(result.Cues) != 1 {
		t.Fatalf("got %d cues, want 1", result.CueCount)
	}
	cue := result.Cues[0]
	if cue.StartMS != 0 || cue.EndMS != 1000 || cue.DurationMS != 1000 {
		t.Fatalf("unexpected timing: %+v", cue)
	}
	if cue.StylePrefix != "{Y:i}" || len(cue.StyleTags) != 1 {
		t.Fatalf("unexpected style: %+v", cue.StyleTags)
	}
	if cue.StyleTags[0].Name != "Y" || cue.StyleTags[0].Value != "i" {
		t.Fatalf("unexpected style tag: %+v", cue.StyleTags[0])
	}
	if cue.RawText != "{Y:i}Hello|world" {
		t.Fatalf("raw text changed: %q", cue.RawText)
	}
	if cue.Text != "Hello\nworld" {
		t.Fatalf("decoded text = %q, want %q", cue.Text, "Hello\nworld")
	}
	if cue.RawLine != "{0}{25}{Y:i}Hello|world" {
		t.Fatalf("raw line changed: %q", cue.RawLine)
	}
}

func TestConvertMultipleStylesAndEmptyLine(t *testing.T) {
	result, err := Convert([]byte("{10}{20}{Y:i}{c:$ff0000}Hi||there\n{20}{30}{note}literal\n"), 10)
	if err != nil {
		t.Fatalf("Convert returned error: %v", err)
	}
	first := result.Cues[0]
	if first.StylePrefix != "{Y:i}{c:$ff0000}" || len(first.StyleTags) != 2 {
		t.Fatalf("unexpected prefix: %q tags=%+v", first.StylePrefix, first.StyleTags)
	}
	if first.Text != "Hi\n\nthere" {
		t.Fatalf("decoded text = %q", first.Text)
	}
	second := result.Cues[1]
	if second.StylePrefix != "" || second.Text != "{note}literal" {
		t.Fatalf("non-style braces were parsed as a style: %+v", second)
	}
}

func TestConvertRoundsToNearestMillisecond(t *testing.T) {
	result, err := Convert([]byte("{1}{24}x"), 23.976)
	if err != nil {
		t.Fatalf("Convert returned error: %v", err)
	}
	cue := result.Cues[0]
	if cue.StartMS != 42 || cue.EndMS != 1001 {
		t.Fatalf("timing = %d..%d ms, want 42..1001", cue.StartMS, cue.EndMS)
	}
	if cue.DurationMS != 959 {
		t.Fatalf("duration = %d, want 959", cue.DurationMS)
	}
}

func TestConvertReportsNegativeInterval(t *testing.T) {
	result, err := Convert([]byte("{30}{10}backwards\n"), 25)
	if err != nil {
		t.Fatalf("Convert returned error: %v", err)
	}
	if result.IssueCount != 1 || len(result.Cues[0].Issues) != 1 {
		t.Fatalf("issues = %+v", result.Issues)
	}
	if got := result.Cues[0].Issues[0].Code; got != "negative_interval" {
		t.Fatalf("issue code = %q, want negative_interval", got)
	}
	if result.Cues[0].DurationMS != -800 {
		t.Fatalf("duration = %d, want -800", result.Cues[0].DurationMS)
	}
}

func TestConvertReportsOverlapOnce(t *testing.T) {
	result, err := Convert([]byte("{0}{50}A\n{40}{60}B\n{60}{70}C\n"), 20)
	if err != nil {
		t.Fatalf("Convert returned error: %v", err)
	}
	if result.CueCount != 3 {
		t.Fatalf("cue count = %d, want 3", result.CueCount)
	}
	if result.IssueCount != 1 {
		t.Fatalf("issue count = %d, want 1: %+v", result.IssueCount, result.Issues)
	}
	issue := result.Issues[0]
	if issue.Code != "overlap" || issue.Cue != 2 || issue.RelatedCue != 1 {
		t.Fatalf("unexpected overlap issue: %+v", issue)
	}
	if !strings.Contains(issue.Message, "2000 ms") || !strings.Contains(issue.Message, "2500 ms") {
		t.Fatalf("overlap message lacks times: %q", issue.Message)
	}
}

func TestConvertReportsMalformedAndNegativeCues(t *testing.T) {
	result, err := Convert([]byte("not a cue\n{-1}{0}negative\n{0}{1}ok\n"), 25)
	if err != nil {
		t.Fatalf("Convert returned error: %v", err)
	}
	if result.CueCount != 2 {
		t.Fatalf("cue count = %d, want 2", result.CueCount)
	}
	codes := make(map[string]bool)
	for _, issue := range result.Issues {
		codes[issue.Code] = true
	}
	if !codes["invalid_timing"] || !codes["negative_time"] {
		t.Fatalf("missing issues: %+v", result.Issues)
	}
	if result.Cues[0].Text != "negative" || result.Cues[1].Text != "ok" {
		t.Fatalf("cue text was not preserved: %+v", result.Cues)
	}
}

func TestConvertRejectsInvalidUTF8(t *testing.T) {
	input := []byte("{0}{1}ok\nbad\xfftext")
	_, err := Convert(input, 25)
	if err == nil {
		t.Fatal("Convert accepted invalid UTF-8")
	}
	var encodingErr *EncodingError
	if !errors.As(err, &encodingErr) {
		t.Fatalf("error type = %T, want *EncodingError", err)
	}
	if encodingErr.Offset != 12 || encodingErr.Line != 2 {
		t.Fatalf("encoding error = %+v, want offset 12 line 2", encodingErr)
	}
}

func TestConvertRejectsInvalidFPS(t *testing.T) {
	for _, fps := range []float64{0, -1} {
		if _, err := Convert([]byte("{0}{1}x"), fps); err == nil {
			t.Fatalf("Convert accepted fps %v", fps)
		}
	}
}
