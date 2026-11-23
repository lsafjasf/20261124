package otio

import (
	"bytes"
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestCrossFrameRateRoundTripAndEdit(t *testing.T) {
	timeline, _, err := Load("testdata/cross_frame_rate.json")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(timeline.Tracks.Tracks()) != 2 {
		t.Fatalf("got %d tracks, want 2", len(timeline.Tracks.Tracks()))
	}
	clip := timeline.ClipByName("film-at-24")
	if clip == nil {
		t.Fatal("film-at-24 clip not found")
	}
	if clip.SourceRange.Start.Rate != 24 || clip.SourceRange.Duration.Rate != 24 {
		t.Fatalf("source rates changed during import: %+v", clip.SourceRange)
	}
	if err := clip.TrimSourceStart(500 * time.Millisecond); err != nil {
		t.Fatalf("TrimSourceStart: %v", err)
	}
	if clip.SourceRange.Start.Value != 36 {
		t.Errorf("start = %v, want 36 source frames", clip.SourceRange.Start.Value)
	}
	if clip.SourceRange.Duration.Value != 60 {
		t.Errorf("duration = %v, want 60 source frames", clip.SourceRange.Duration.Value)
	}

	marker := timeline.TrackByName("V1").Markers[0]
	if marker.MarkedRange.Start.Rate != 23.976 || marker.MarkedRange.Duration.Rate != 24 {
		t.Fatalf("marker rates changed: %+v", marker.MarkedRange)
	}
	roundTripped := assertRoundTrip(t, timeline)
	got := roundTripped.ClipByName("film-at-24")
	if got == nil || got.SourceRange.Start.Value != 36 || got.SourceRange.Duration.Value != 60 {
		t.Fatalf("edited source range did not survive round trip: %+v", got)
	}
	if got.SourceRange.Start.Rate != 24 || got.SourceRange.Duration.Rate != 24 {
		t.Fatalf("edited source rates were normalized: %+v", got.SourceRange)
	}
	if gotMarker := roundTripped.TrackByName("V1").Markers[0]; gotMarker.MarkedRange.Start.Rate != 23.976 {
		t.Fatalf("cross-rate marker rate was lost: %+v", gotMarker.MarkedRange)
	}
}

func TestRationalTimeComparisonAcrossRates(t *testing.T) {
	at24 := NewRationalTime(24, 24)
	at30 := NewRationalTime(30, 30)
	equal, err := at24.Compare(at30)
	if err != nil {
		t.Fatal(err)
	}
	if equal != 0 {
		t.Fatalf("24@24 and 30@30 compare as %d, want equal", equal)
	}
	rescaled, err := at24.Rescaled(30)
	if err != nil {
		t.Fatal(err)
	}
	if rescaled.Value != 30 || rescaled.Rate != 30 {
		t.Fatalf("rescaled = %+v, want 30 frames at 30 fps", rescaled)
	}
}

func TestOverlappingMarkersArePreserved(t *testing.T) {
	timeline, report, err := Load("testdata/overlapping_markers.json")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	clip := timeline.ClipByName("marked-shot")
	if clip == nil {
		t.Fatal("marked-shot clip not found")
	}
	pairs, err := clip.OverlappingMarkers()
	if err != nil {
		t.Fatalf("OverlappingMarkers: %v", err)
	}
	if len(pairs) != 2 {
		t.Fatalf("overlap pairs = %v, want 2; markers must not be merged", pairs)
	}
	if len(clip.Markers) != 4 {
		t.Fatalf("marker count = %d, want 4", len(clip.Markers))
	}
	for _, unknown := range report.UnknownFields {
		if unknown.Action != ActionRetained {
			t.Fatalf("field %s was not retained: %+v", unknown.Path, unknown)
		}
	}
	roundTripped := assertRoundTrip(t, timeline)
	got := roundTripped.ClipByName("marked-shot")
	if len(got.Markers) != 4 {
		t.Fatalf("marker count after round trip = %d, want 4", len(got.Markers))
	}
	if got.Markers[1].MarkedRange.Start.Rate != 30 || got.Markers[1].MarkedRange.Duration.Rate != 29.97 {
		t.Fatalf("mixed marker rates were normalized: %+v", got.Markers[1].MarkedRange)
	}
}

func TestMissingMediaIsReportedAndRoundTrips(t *testing.T) {
	timeline, report, err := Load("testdata/missing_media.json")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(report.MissingMedia) != 4 {
		t.Fatalf("missing media count = %d, want 4: %+v", len(report.MissingMedia), report.MissingMedia)
	}
	gotClips := map[string]bool{}
	for _, missing := range report.MissingMedia {
		gotClips[missing.Clip] = true
	}
	for _, name := range []string{"no-reference", "null-reference", "empty-target", "empty-reference-map"} {
		if !gotClips[name] {
			t.Errorf("clip %q was not named in missing media report", name)
		}
	}
	if gotClips["available-target"] {
		t.Errorf("available-target was incorrectly reported as missing")
	}
	assertUnknownFieldReported(t, report, "media_reference")
	assertUnknownFieldReported(t, report, "media_references")

	roundTripped := assertRoundTrip(t, timeline)
	emptyTarget := roundTripped.ClipByName("empty-target")
	if emptyTarget == nil {
		t.Fatal("empty-target disappeared")
	}
	reference, ok := emptyTarget.Extra["media_reference"].(map[string]any)
	if !ok || reference["target_url"] != "" {
		t.Fatalf("missing media reference was not retained: %#v", emptyTarget.Extra)
	}
	available := roundTripped.ClipByName("available-target")
	if available == nil || available.Extra["active_media_reference_key"] != "main" {
		t.Fatalf("available media metadata was not retained: %#v", available)
	}
}

func TestUnknownFieldsAndObjectsRoundTrip(t *testing.T) {
	timeline, report, err := Load("testdata/unknown_fields.json")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	for _, field := range []string{
		"timebase",
		"custom_clip_field",
		"custom_range_field",
		"custom_time_field",
		"custom_marker_field",
		"media_reference",
	} {
		assertUnknownFieldReported(t, report, field)
	}
	if len(report.UnknownObjects) != 1 || report.UnknownObjects[0].Schema != "Transition.1" {
		t.Fatalf("unknown object report = %+v, want Transition.1", report.UnknownObjects)
	}

	roundTripped := assertRoundTrip(t, timeline)
	clip := roundTripped.ClipByName("retained-clip")
	if clip == nil {
		t.Fatal("retained-clip disappeared")
	}
	custom, ok := clip.Extra["custom_clip_field"].(map[string]any)
	if !ok {
		t.Fatalf("custom clip field was lost: %#v", clip.Extra)
	}
	nested, ok := custom["nested"].([]any)
	if !ok || len(nested) != 3 {
		t.Fatalf("nested custom data was lost: %#v", custom)
	}
	if clip.SourceRange.Extra["custom_range_field"] != "keep-me" {
		t.Fatalf("time range extra was lost: %#v", clip.SourceRange.Extra)
	}
	if fmt.Sprint(clip.SourceRange.Start.Extra["custom_time_field"]) != "1.5" {
		t.Fatalf("rational time extra was lost: %#v", clip.SourceRange.Start.Extra)
	}
	if clip.Markers[0].Extra["custom_marker_field"] != "keep-marker" {
		t.Fatalf("marker extra was lost: %#v", clip.Markers[0].Extra)
	}
	transition := roundTripped.TrackByName("V1").Children[1].(*Opaque)
	if transition.Raw["custom_transition_field"] != "retain-verbatim" {
		t.Fatalf("opaque object was not retained verbatim: %#v", transition.Raw)
	}
}

func TestEditAddTrackGapAndMarkerThenRoundTrip(t *testing.T) {
	timeline, _, err := Load("testdata/unknown_fields.json")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	score := NewTrack("A1")
	if err := score.Append(NewGap("lead-in", NewRationalTime(25, 25))); err != nil {
		t.Fatal(err)
	}
	clip := NewClip("music", NewTimeRange(NewRationalTime(30, 60), NewRationalTime(30, 150)))
	if err := score.Append(clip); err != nil {
		t.Fatal(err)
	}
	marked := NewMarker("music cue", "PURPLE", NewTimeRange(NewRationalTime(24, 24), NewRationalTime(24, 12)))
	score.AddMarker(marked)
	if err := timeline.AppendTrack(score); err != nil {
		t.Fatal(err)
	}
	if got := timeline.ClipByName("retained-clip"); got == nil || got.SourceRange == nil {
		t.Fatal("existing timeline was damaged while editing")
	}

	roundTripped := assertRoundTrip(t, timeline)
	if len(roundTripped.Tracks.Tracks()) != 2 {
		t.Fatalf("track count after edit = %d, want 2", len(roundTripped.Tracks.Tracks()))
	}
	got := roundTripped.ClipByName("music")
	if got == nil || got.SourceRange.Duration.Rate != 30 || got.SourceRange.Duration.Value != 150 {
		t.Fatalf("new clip did not round trip: %+v", got)
	}
	if roundTripped.ClipByName("retained-clip").Extra["custom_clip_field"] == nil {
		t.Fatal("unknown fields were lost while editing and re-exporting")
	}
}

func TestJSONMarshalRoundTrip(t *testing.T) {
	timeline, _, err := Load("testdata/overlapping_markers.json")
	if err != nil {
		t.Fatal(err)
	}
	data, err := timeline.MarshalJSON()
	if err != nil {
		t.Fatal(err)
	}
	parsed, _, err := Decode(data)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(timeline, parsed) {
		t.Fatal("json.Marshaler round trip changed the model")
	}
}

func TestNullKnownFieldsArePreserved(t *testing.T) {
	data := []byte(`{
	  "OTIO_SCHEMA": "Timeline.1",
	  "name": "null-fields",
	  "global_start_time": null,
	  "markers": null,
	  "tracks": {
	    "OTIO_SCHEMA": "Stack.1",
	    "name": "tracks",
	    "children": [
	      {
	        "OTIO_SCHEMA": "Track.1",
	        "name": "V1",
	        "children": [
	          {
	            "OTIO_SCHEMA": "Clip.2",
	            "name": "clip-with-null-range",
	            "source_range": null,
	            "markers": [
	              {
	                "OTIO_SCHEMA": "Marker.2",
	                "name": "null-range",
	                "marked_range": null
	              }
	            ]
	          }
	        ]
	      }
	    ]
	  }
	}`)
	timeline, _, err := Decode(data)
	if err != nil {
		t.Fatal(err)
	}
	roundTripped := assertRoundTrip(t, timeline)
	clip := roundTripped.ClipByName("clip-with-null-range")
	if clip == nil || !clip.sourceRangeSet || clip.SourceRange != nil {
		t.Fatalf("null source_range was not preserved: %+v", clip)
	}
	if !clip.Markers[0].rangeSet || clip.Markers[0].MarkedRange != nil {
		t.Fatalf("null marker range was not preserved: %+v", clip.Markers[0])
	}
	if !roundTripped.globalStartSet || roundTripped.GlobalStartTime != nil {
		t.Fatalf("null global_start_time was not preserved: %+v", roundTripped.GlobalStartTime)
	}
}

func assertRoundTrip(t *testing.T, timeline *Timeline) *Timeline {
	t.Helper()
	data, report, err := Encode(timeline)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	if !bytes.HasSuffix(data, []byte("\n")) {
		t.Fatal("Encode output does not end with a newline")
	}
	parsed, parsedReport, err := Decode(data)
	if err != nil {
		t.Fatalf("Decode exported JSON: %v\n%s", err, data)
	}
	if !reflect.DeepEqual(timeline, parsed) {
		t.Fatalf("model changed across round trip\nbefore: %#v\nafter:  %#v", timeline, parsed)
	}
	if len(report.UnknownFields) != len(parsedReport.UnknownFields) {
		t.Fatalf("unknown field count changed across round trip: %d vs %d", len(report.UnknownFields), len(parsedReport.UnknownFields))
	}
	return parsed
}

func assertUnknownFieldReported(t *testing.T, report *Report, field string) {
	t.Helper()
	for _, unknown := range report.UnknownFields {
		if unknown.Field == field {
			if unknown.Action != ActionRetained {
				t.Fatalf("field %q action = %q, want %q", field, unknown.Action, ActionRetained)
			}
			if !strings.HasPrefix(unknown.Path, "/") {
				t.Fatalf("field %q has non-JSON-Pointer path %q", field, unknown.Path)
			}
			return
		}
	}
	t.Fatalf("field %q was not named in report: %+v", field, report.UnknownFields)
}

func TestMain(m *testing.M) {
	if _, err := os.Stat("testdata"); err != nil {
		panic("tests must run from the package directory")
	}
	os.Exit(m.Run())
}
