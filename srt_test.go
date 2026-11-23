package subtime

import (
	"bytes"
	"strings"
	"testing"
)

const duplicateSRT = `1
00:00:01,000 --> 00:00:02,000
first

2
00:00:01,000 --> 00:00:02,000
second

3
00:59:59,990 --> 01:00:00,010
hour
`

func TestParseSRTPreservesDuplicateTimestamps(t *testing.T) {
	cues, err := ParseSRT(strings.NewReader(duplicateSRT))
	if err != nil {
		t.Fatal(err)
	}
	if len(cues) != 3 {
		t.Fatalf("got %d cues", len(cues))
	}
	if !cues[0].Start.Equal(cues[1].Start) || !cues[0].End.Equal(cues[1].End) {
		t.Fatalf("duplicate timestamps changed during parsing")
	}
	if cues[0].Text != "first" || cues[1].Text != "second" {
		t.Fatalf("cue text/order changed: %+v", cues)
	}
}

func TestParseSRTAcceptsBOM(t *testing.T) {
	cues, err := ParseSRT(strings.NewReader("\ufeff1\n00:00:01,000 --> 00:00:02,000\nx\n"))
	if err != nil {
		t.Fatal(err)
	}
	if len(cues) != 1 || cues[0].Number != 1 {
		t.Fatalf("cues = %+v", cues)
	}
}

func TestMapSRTThenWriteSRT(t *testing.T) {
	cues, err := ParseSRT(strings.NewReader(duplicateSRT))
	if err != nil {
		t.Fatal(err)
	}
	mapped, err := MapSRTCues(cues, Rate25, Rate2997DF, IntervalPolicy{
		Start: SnapNearest, End: SnapNearest, EnsureOneFrame: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(mapped) != 3 {
		t.Fatalf("got %d mapped cues", len(mapped))
	}
	if !mapped[0].Mapping.Start.Position.Equal(mapped[1].Mapping.Start.Position) {
		t.Fatalf("duplicate starts diverged")
	}

	var output bytes.Buffer
	if err := WriteSRT(&output, []SRTCue{
		mapped[0].Output, mapped[1].Output, mapped[2].Output,
	}, SRTFormatOptions{MillisecondSnap: SnapNearest, EnsureEndAfterStart: true}); err != nil {
		t.Fatal(err)
	}
	reparsed, err := ParseSRT(bytes.NewReader(output.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	if len(reparsed) != 3 {
		t.Fatalf("reparsed %d cues:\n%s", len(reparsed), output.String())
	}
	for i := range mapped {
		if reparsed[i].Number != mapped[i].Output.Number {
			t.Fatalf("cue number changed at %d", i)
		}
	}
}

func TestMapSRTThroughExactRateChain(t *testing.T) {
	cues := []SRTCue{{
		Number: 1,
		Start:  NewRational(7200, 1),
		End:    NewRational(7201, 1),
		Text:   "hour",
	}}
	mapped, err := MapSRTCuesThrough(
		cues, Rate23976, []Rate{Rate25, Rate2997DF}, IntervalPolicy{
			Start: SnapFloor, End: SnapCeil, EnsureOneFrame: true,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	startSeconds, _ := mapped[0].Mapping.Start.Seconds()
	endSeconds, _ := mapped[0].Mapping.End.Seconds()
	if !startSeconds.Equal(cues[0].Start) || !endSeconds.Equal(cues[0].End) {
		t.Fatalf("exact target times changed: %s -> %s", startSeconds, endSeconds)
	}
	if mapped[0].Mapping.Source.ID != "1" {
		t.Fatalf("original source was replaced: %+v", mapped[0].Mapping.Source)
	}
}

func TestWriteSRTRequiresExplicitMillisecondPolicy(t *testing.T) {
	cues := []SRTCue{{Number: 1, Start: NewRational(1, 1), End: NewRational(2, 1), Text: "x"}}
	if err := WriteSRT(&bytes.Buffer{}, cues, SRTFormatOptions{}); err == nil {
		t.Fatal("missing millisecond policy should fail")
	}
}

func TestProjectIntervalChainKeepsExactPoints(t *testing.T) {
	start := Point{Rate: Rate23976, Position: NewRational(24000, 1)}
	end := Point{Rate: Rate23976, Position: NewRational(24024, 1)}
	projected, err := ProjectInterval(Interval{Start: start, End: end}, Rate25, Rate2997DF, Rate25)
	if err != nil {
		t.Fatal(err)
	}
	back, err := ProjectInterval(projected, Rate23976)
	if err != nil {
		t.Fatal(err)
	}
	if back.Start.Position.Cmp(start.Position) != 0 || back.End.Position.Cmp(end.Position) != 0 {
		t.Fatalf("projection changed points: %+v -> %+v", start, back.Start)
	}
}
