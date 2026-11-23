package edl

import (
	"bytes"
	"os"
	"reflect"
	"strings"
	"testing"
)

func loadFixture(t *testing.T, path string) (*FileTimeline, *EDLFile) {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	parsed, err := ParseEDL(f, path, ParseOptions{Rate: Rate2997})
	if err != nil {
		t.Fatalf("ParseEDL(%s): %v", path, err)
	}
	timeline, err := BuildTimeline(parsed)
	if err != nil {
		t.Fatalf("BuildTimeline(%s): %v", path, err)
	}
	return timeline, parsed
}

func TestDFMidnightTimeline(t *testing.T) {
	timeline, _ := loadFixture(t, "testdata/df_midnight.edl")
	if timeline.Mode != DF || timeline.MixedModes {
		t.Fatalf("mode = %s, mixed = %v", timeline.Mode, timeline.MixedModes)
	}
	if timeline.TotalFrames != 1200 {
		t.Errorf("total frames = %d, want 1200", timeline.TotalFrames)
	}
	if timeline.SumEventFrames != 1200 {
		t.Errorf("sum event frames = %d, want 1200", timeline.SumEventFrames)
	}
	if timeline.FirstCut.Frame != 0 || timeline.LastCut.Frame != 1200 {
		t.Errorf("cuts = %d..%d, want 0..1200", timeline.FirstCut.Frame, timeline.LastCut.Frame)
	}
	if timeline.FirstCut.AbsoluteFrame != 2589108 {
		t.Errorf("first absolute frame = %d, want 2589108", timeline.FirstCut.AbsoluteFrame)
	}
	wantCuts := []int64{0, 600, 900, 1200}
	if !reflect.DeepEqual(timeline.CutFrames, wantCuts) {
		t.Errorf("cut frames = %v, want %v", timeline.CutFrames, wantCuts)
	}
	if got := timeline.Events[0].SourceDurationFrames; got != 600 {
		t.Errorf("event 1 source duration = %d, want 600", got)
	}
	if got := timeline.Events[1].SourceInAbs; got != 300 {
		t.Errorf("event 2 source in = %d, want 300", got)
	}
	if !reflect.DeepEqual(timeline.RepeatedReels, []string{"TAPE_A (2)"}) {
		t.Errorf("repeated reels = %v", timeline.RepeatedReels)
	}
	if len(timeline.LostTransitions) != 1 || timeline.LostTransitions[0].Code != "D" {
		t.Errorf("lost transitions = %+v", timeline.LostTransitions)
	}
	if !timeline.Validation.ContinuityWithinTrack || len(timeline.Validation.Issues) != 0 {
		t.Errorf("validation = %+v", timeline.Validation)
	}
}

func TestNDFReusedReelAndNegativeSourceOffset(t *testing.T) {
	timeline, _ := loadFixture(t, "testdata/ndf_reel_reuse.edl")
	if timeline.Mode != NDF || timeline.MixedModes {
		t.Fatalf("mode = %s, mixed = %v", timeline.Mode, timeline.MixedModes)
	}
	if timeline.TotalFrames != 450 || timeline.LastCut.Frame != 450 {
		t.Errorf("total/last = %d/%d, want 450/450", timeline.TotalFrames, timeline.LastCut.Frame)
	}
	if got := timeline.Events[0].SourceRecordOffset; got != -9000 {
		t.Errorf("source-to-record offset = %d, want -9000", got)
	}
	if !reflect.DeepEqual(timeline.RepeatedReels, []string{"REEL_X (2)"}) {
		t.Errorf("repeated reels = %v", timeline.RepeatedReels)
	}
	if len(timeline.LostTransitions) != 2 {
		t.Fatalf("lost transitions = %+v", timeline.LostTransitions)
	}
	if timeline.LostTransitions[0].Class != "wipe" || timeline.LostTransitions[1].Class != "key" {
		t.Errorf("transition classes = %+v", timeline.LostTransitions)
	}
}

func TestSignedNegativeTimecodeTimeline(t *testing.T) {
	timeline, _ := loadFixture(t, "testdata/negative_offset.edl")
	if timeline.TotalFrames != 300 || timeline.FirstCut.AbsoluteFrame != -60 {
		t.Errorf("total/first = %d/%d, want 300/-60", timeline.TotalFrames, timeline.FirstCut.AbsoluteFrame)
	}
	if timeline.FirstCut.Timecode != "-00:00:02;00" {
		t.Errorf("first timecode = %q", timeline.FirstCut.Timecode)
	}
	if timeline.Events[0].TimelineIn != 0 || timeline.Events[1].TimelineIn != 150 {
		t.Errorf("timeline placements = %d,%d", timeline.Events[0].TimelineIn, timeline.Events[1].TimelineIn)
	}
}

func TestFrameAccurateMapping(t *testing.T) {
	timeline, _ := loadFixture(t, "testdata/df_midnight.edl")
	tests := []struct {
		frame  int64
		event  int
		source int64
		tc     string
	}{
		{0, 1, 2589108, "23:59:50;00"},
		{599, 1, 2589707, "24:00:09;29"},
		{600, 2, 300, "00:00:10;00"},
		{900, 3, 1800, "00:01:00;02"},
		{1199, 3, 2099, "00:01:10;01"},
	}
	for _, tt := range tests {
		got, err := timeline.FrameAt("V", tt.frame)
		if err != nil {
			t.Fatalf("FrameAt(%d): %v", tt.frame, err)
		}
		if got.Event != tt.event || got.SourceFrame != tt.source || got.UnwrappedSourceTC != tt.tc {
			t.Errorf("FrameAt(%d) = event %d source %d %s; want event %d source %d %s",
				tt.frame, got.Event, got.SourceFrame, got.UnwrappedSourceTC,
				tt.event, tt.source, tt.tc)
		}
	}
	if _, err := timeline.FrameAt("V", 1200); err == nil {
		t.Error("frame at exclusive end unexpectedly mapped")
	}
}

func TestCutOnlyExportRoundtrip(t *testing.T) {
	timeline, _ := loadFixture(t, "testdata/df_midnight.edl")
	var buf bytes.Buffer
	if err := ExportEDL(&buf, timeline); err != nil {
		t.Fatal(err)
	}
	parsed, err := ParseEDL(bytes.NewReader(buf.Bytes()), "roundtrip.edl", ParseOptions{Rate: Rate2997})
	if err != nil {
		t.Fatalf("reparse exported EDL: %v\n%s", err, buf.String())
	}
	roundtripped, err := BuildTimeline(parsed)
	if err != nil {
		t.Fatal(err)
	}
	result := CompareCutRoundtrip(timeline, roundtripped)
	if !result.OK {
		t.Fatalf("roundtrip mismatches: %v\n%s", result.Mismatches, buf.String())
	}
	if len(roundtripped.LostTransitions) != 0 {
		t.Errorf("exported hard cut should no longer be a lost transition: %+v", roundtripped.LostTransitions)
	}
}

func TestBatchSummaryAndJSON(t *testing.T) {
	paths := []string{
		"testdata/df_midnight.edl",
		"testdata/ndf_reel_reuse.edl",
		"testdata/negative_offset.edl",
	}
	var files []*EDLFile
	for _, path := range paths {
		parsed, err := ParseFile(path, ParseOptions{Rate: Rate2997})
		if err != nil {
			t.Fatal(err)
		}
		files = append(files, parsed)
	}
	batch, err := BuildBatch(files)
	if err != nil {
		t.Fatal(err)
	}
	if batch.Summary.FileCount != 3 || batch.Summary.TotalFrames != 1950 {
		t.Errorf("summary = %+v", batch.Summary)
	}
	if batch.Summary.TotalDuration.Rational != "13013/200" ||
		batch.Summary.TotalDuration.Seconds != "65.065" {
		t.Errorf("duration = %+v", batch.Summary.TotalDuration)
	}
	if batch.Summary.UnsupportedTransition != 3 || batch.Summary.ValidationIssues != 0 {
		t.Errorf("summary = %+v", batch.Summary)
	}
	data, err := MarshalBatch(batch)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	if !strings.Contains(text, `"text": "23:59:50;00"`) ||
		!strings.Contains(text, `"fps": "29.97"`) {
		t.Errorf("timecode JSON is not readable:\n%s", text[:min(len(text), 800)])
	}
	if strings.Contains(text, `"Rate": {}`) {
		t.Error("JSON leaked the internal Rate struct")
	}
}

func TestFCMHeaderCanChangeModeMidFile(t *testing.T) {
	input := `TITLE: MODE CHANGE
FCM: DROP FRAME
001 A V C 00:00:00;00 00:00:01;00 00:00:00;00 00:00:01;00
FCM: NON-DROP FRAME
002 A V C 00:00:01:00 00:00:02:00 00:00:01:00 00:00:02:00
`
	parsed, err := ParseEDL(strings.NewReader(input), "mixed.edl", ParseOptions{Rate: Rate2997})
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Events[0].SourceIn.Mode != DF || parsed.Events[1].SourceIn.Mode != NDF {
		t.Errorf("modes = %s, %s", parsed.Events[0].SourceIn.Mode, parsed.Events[1].SourceIn.Mode)
	}
	if !parsed.MixedModes {
		t.Error("mixed mode file not identified")
	}
	timeline, err := BuildTimeline(parsed)
	if err != nil {
		t.Fatal(err)
	}
	if timeline.TotalFrames != 60 || timeline.LastCut.Frame != 60 {
		t.Errorf("total/last = %d/%d, want 60/60", timeline.TotalFrames, timeline.LastCut.Frame)
	}
}

func TestMixedModeExportKeepsNDFLines(t *testing.T) {
	input := `TITLE: MIXED
FCM: DROP FRAME
001 A V C 00:00:00;00 00:00:01;00 00:00:00;00 00:00:01;00
002 A V C 00:00:01;00 00:00:02;00 00:00:01;00 00:00:02;00
FCM: NON-DROP FRAME
003 A V C 00:00:02:00 00:00:03:00 00:00:02:00 00:00:03:00
`
	parsed, err := ParseEDL(strings.NewReader(input), "mixed-export.edl", ParseOptions{Rate: Rate2997})
	if err != nil {
		t.Fatal(err)
	}
	original, err := BuildTimeline(parsed)
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if err := ExportEDL(&buf, original); err != nil {
		t.Fatal(err)
	}
	reparsed, err := ParseEDL(bytes.NewReader(buf.Bytes()), "mixed-export-roundtrip.edl", ParseOptions{Rate: Rate2997})
	if err != nil {
		t.Fatalf("reparse: %v\n%s", err, buf.String())
	}
	roundtrip, err := BuildTimeline(reparsed)
	if err != nil {
		t.Fatal(err)
	}
	if got := roundtrip.Events[2].SourceIn.Mode; got != NDF {
		t.Errorf("third event mode = %s, want NDF\n%s", got, buf.String())
	}
	if result := CompareCutRoundtrip(original, roundtrip); !result.OK {
		t.Errorf("mismatches: %v\n%s", result.Mismatches, buf.String())
	}
}
