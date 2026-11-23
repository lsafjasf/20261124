package timeline

import (
	"bytes"
	"math/big"
	"testing"
)

func TestMixedFPSImportUsesThreeTimeDomains(t *testing.T) {
	tl, err := ImportFile("testdata/mixed-fps.json")
	if err != nil {
		t.Fatal(err)
	}
	if got, want := ratString(tl.SequenceFPS), "25"; got != want {
		t.Fatalf("sequence fps = %s, want %s", got, want)
	}
	if got, want := ratString(tl.Media["cam-a"].FPS), "24000/1001"; got != want {
		t.Fatalf("cam-a fps = %s, want %s", got, want)
	}
	if got, want := len(tl.Events), 10; got != want {
		t.Fatalf("events = %d, want %d", got, want)
	}

	gap := tl.Events[1]
	if gap.Kind != "gap" || gap.Source != nil || gap.MediaIn != nil {
		t.Fatalf("gap leaked into media/source domain: %#v", gap)
	}
	if got, want := ratString(gap.SequenceDuration), "7/25"; got != want {
		t.Fatalf("gap duration = %s, want %s", got, want)
	}

	clip := tl.Events[6]
	if got, want := ratString(clip.MediaIn), "1001/100"; got != want {
		t.Fatalf("media in = %s, want %s", got, want)
	}
	if got, want := ratString(clip.SourceOut), "1001/10"; got != want {
		t.Fatalf("source duration = %s, want %s", got, want)
	}
	if got, want := ratString(clip.SequenceOutFrames), "30199/12"; got != want {
		t.Fatalf("sequence out = %s frames, want %s", got, want)
	}
	if got, want := ratString(tl.Duration), "8651/60"; got != want {
		t.Fatalf("duration = %s s, want %s", got, want)
	}
}

func TestRoundTripAndCutErrors(t *testing.T) {
	original, err := ImportFile("testdata/mixed-fps.json")
	if err != nil {
		t.Fatal(err)
	}
	data, err := RoundTripBytes(original)
	if err != nil {
		t.Fatal(err)
	}
	reimported, err := Import(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	if err := VerifyRoundTrip(original, reimported); err != nil {
		t.Fatalf("round-trip: %v", err)
	}

	rows, err := BuildCutErrors(original, reimported)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := len(rows), 20; got != want {
		t.Fatalf("cut rows = %d, want %d", got, want)
	}
	half := big.NewRat(1, 2)
	for _, row := range rows {
		frameErr, ok := new(big.Rat).SetString(row.FrameError)
		if !ok || absRat(frameErr).Cmp(half) > 0 {
			t.Errorf("%s %s: frame error %s exceeds half a frame", row.EventID, row.Boundary, row.FrameError)
		}
		if row.RoundTripErrorSeconds != "0.000000000000" {
			t.Errorf("%s %s: round-trip error %s", row.EventID, row.Boundary, row.RoundTripErrorSeconds)
		}
	}

	summary := Summarize(original, rows)
	if !summary.RoundTripExact {
		t.Fatal("summary says round-trip is not exact")
	}
	if got, want := summary.LegacyDurationFrames, "3568"; got != want {
		t.Fatalf("legacy duration = %s frames, want %s", got, want)
	}
	if got, want := summary.LegacyEndErrorFrames, "-36.583333333"; got != want {
		t.Fatalf("legacy end error = %s frames, want %s", got, want)
	}
}

func TestRateSpellingsAreExact(t *testing.T) {
	cases := map[string]string{
		"23.976":     "24000/1001",
		"24000/1001": "24000/1001",
		"25":         "25",
		"30":         "30",
		"29.97":      "30000/1001",
	}
	for input, want := range cases {
		got, err := ParseRate(input)
		if err != nil {
			t.Errorf("ParseRate(%q): %v", input, err)
			continue
		}
		if ratString(got) != want {
			t.Errorf("ParseRate(%q) = %s, want %s", input, ratString(got), want)
		}
	}
}

func TestInvalidGapAndMediaRangeFail(t *testing.T) {
	doc := Document{
		Version:  DocumentVersion,
		Sequence: Sequence{FPS: "25"},
		Media:    []Media{{ID: "m", FPS: "25", Frames: 10}},
		Events: []Event{
			{ID: "g", Kind: "gap", SequenceFrames: -1},
		},
	}
	if _, err := BuildTimeline(doc); err == nil {
		t.Fatal("negative gap accepted")
	}
	doc.Events = []Event{{ID: "g0", Kind: "gap"}}
	if _, err := BuildTimeline(doc); err == nil {
		t.Fatal("zero-length gap accepted")
	}
	doc.Events = []Event{{ID: "c", Kind: "clip", Media: "m", MediaInFrame: 2, MediaOutFrame: 11}}
	if _, err := BuildTimeline(doc); err == nil {
		t.Fatal("out-of-range clip accepted")
	}
}
