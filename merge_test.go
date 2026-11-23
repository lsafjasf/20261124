package timeline

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestFixtureMerges(t *testing.T) {
	for _, fixture := range []string{
		"cross-track",
		"transition-params",
		"additions",
	} {
		t.Run(fixture, func(t *testing.T) {
			result, expected := mergeFixture(t, fixture)
			if !result.Clean() {
				t.Fatalf("expected clean merge, got conflicts: %#v", result.Conflicts)
			}
			assertTimelineJSON(t, result.Timeline, expected)
		})
	}
}

func TestDifferentClipsAndMarkersAreAbsorbed(t *testing.T) {
	ancestor := Timeline{
		Tracks: []Track{{ID: "V1"}, {ID: "V2"}},
		Clips: []Clip{
			{ID: "clip-a", TrackID: "V1", Start: 0, Duration: 4},
			{ID: "clip-b", TrackID: "V2", Start: 9, Duration: 6},
		},
		Markers: []Marker{
			{ID: "marker-a", Time: 1, Label: "A"},
			{ID: "marker-b", Time: 5, Label: "B"},
		},
	}
	ours := cloneTimeline(t, ancestor)
	ours.Clips[0].Start = 2
	ours.Markers[0].Label = "A changed"

	theirs := cloneTimeline(t, ancestor)
	theirs.Clips[1].Duration = 10
	theirs.Markers[1].Color = "yellow"

	result, err := Merge(ancestor, ours, theirs)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Clean() {
		t.Fatalf("unexpected conflicts: %#v", result.Conflicts)
	}
	if got := result.Timeline.Clips[0].Start; got != 2 {
		t.Fatalf("clip-a start = %v, want 2", got)
	}
	if got := result.Timeline.Clips[1].Duration; got != 10 {
		t.Fatalf("clip-b duration = %v, want 10", got)
	}
	if got := result.Timeline.Markers[0].Label; got != "A changed" {
		t.Fatalf("marker-a label = %q", got)
	}
	if got := result.Timeline.Markers[1].Color; got != "yellow" {
		t.Fatalf("marker-b color = %q", got)
	}
}

func TestSamePropertyConflictKeepsAncestorAndShowsBothSides(t *testing.T) {
	result, expected := mergeFixture(t, "conflict")
	if result.Clean() {
		t.Fatal("expected a conflict")
	}
	if len(result.Conflicts) != 1 {
		t.Fatalf("conflict count = %d, want 1: %#v", len(result.Conflicts), result.Conflicts)
	}

	conflict := result.Conflicts[0]
	if conflict.Path != `clips[id="clip-hero"].start` {
		t.Fatalf("conflict path = %q", conflict.Path)
	}
	assertConflictNumber(t, conflict.Ancestor, "5")
	assertConflictNumber(t, conflict.Ours, "8")
	assertConflictNumber(t, conflict.Theirs, "11")
	assertTimelineJSON(t, result.Timeline, expected)

	var report bytes.Buffer
	if err := FormatConflicts(&report, result.Conflicts); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`CONFLICT clips[id="clip-hero"].start`,
		"ancestor : 5",
		"ours     : 8",
		"theirs   : 11",
	} {
		if !strings.Contains(report.String(), want) {
			t.Fatalf("report missing %q:\n%s", want, report.String())
		}
	}
}

func TestCrossTrackMoveConflictDoesNotUseLatest(t *testing.T) {
	ancestor := Timeline{
		Tracks: []Track{{ID: "V1"}, {ID: "V2"}, {ID: "V3"}},
		Clips:  []Clip{{ID: "clip-a", TrackID: "V1", Start: 0, Duration: 5}},
	}
	ours := cloneTimeline(t, ancestor)
	ours.Clips[0].TrackID = "V2"
	theirs := cloneTimeline(t, ancestor)
	theirs.Clips[0].TrackID = "V3"

	result, err := Merge(ancestor, ours, theirs)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Conflicts) != 1 {
		t.Fatalf("conflict count = %d, want 1", len(result.Conflicts))
	}
	conflict := result.Conflicts[0]
	if conflict.Path != `clips[id="clip-a"].trackId` {
		t.Fatalf("conflict path = %q", conflict.Path)
	}
	if conflict.Ours.Display() != `"V2"` || conflict.Theirs.Display() != `"V3"` {
		t.Fatalf("side values were not preserved: %#v", conflict)
	}
	if got := result.Timeline.Clips[0].TrackID; got != "V1" {
		t.Fatalf("merged trackId = %q, want ancestor value V1", got)
	}
}

func TestTransitionPropertyConflictDisplaysAncestorAndBothSides(t *testing.T) {
	ancestor := Timeline{
		Tracks: []Track{{ID: "V1"}},
		Clips: []Clip{{
			ID:       "clip-a",
			TrackID:  "V1",
			Duration: 5,
			Transitions: []Transition{{
				ID:     "fade-in",
				Kind:   "fade",
				Params: map[string]any{"duration": 500},
			}},
		}},
	}
	ours := cloneTimeline(t, ancestor)
	ours.Clips[0].Transitions[0].Params["duration"] = 750
	theirs := cloneTimeline(t, ancestor)
	theirs.Clips[0].Transitions[0].Params["duration"] = 900

	result, err := Merge(ancestor, ours, theirs)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Conflicts) != 1 {
		t.Fatalf("conflict count = %d, want 1", len(result.Conflicts))
	}
	conflict := result.Conflicts[0]
	wantPath := `clips[id="clip-a"].transitions[id="fade-in"].params.duration`
	if conflict.Path != wantPath {
		t.Fatalf("conflict path = %q, want %q", conflict.Path, wantPath)
	}
	assertConflictNumber(t, conflict.Ancestor, "500")
	assertConflictNumber(t, conflict.Ours, "750")
	assertConflictNumber(t, conflict.Theirs, "900")
}

func TestEntityAdditionAndPropertyEditMerge(t *testing.T) {
	ancestor := Timeline{
		Tracks: []Track{{ID: "V1"}, {ID: "V2"}},
		Clips:  []Clip{{ID: "clip-a", TrackID: "V1", Start: 0, Duration: 5}},
	}
	ours := cloneTimeline(t, ancestor)
	ours.Clips = append(ours.Clips, Clip{ID: "clip-new", TrackID: "V2", Start: 6, Duration: 2})
	theirs := cloneTimeline(t, ancestor)
	theirs.Clips[0].Duration = 8

	result, err := Merge(ancestor, ours, theirs)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Clean() {
		t.Fatalf("unexpected conflicts: %#v", result.Conflicts)
	}
	if len(result.Timeline.Clips) != 2 {
		t.Fatalf("clip count = %d, want 2", len(result.Timeline.Clips))
	}
	if result.Timeline.Clips[0].Duration != 8 {
		t.Fatalf("clip duration = %v, want 8", result.Timeline.Clips[0].Duration)
	}
}

func mergeFixture(t *testing.T, name string) (MergeResult, Timeline) {
	t.Helper()
	read := func(file string) []byte {
		t.Helper()
		data, err := os.ReadFile(filepath.Join("testdata", name, file))
		if err != nil {
			t.Fatal(err)
		}
		return data
	}
	result, err := MergeJSON(read("base.json"), read("ours.json"), read("theirs.json"))
	if err != nil {
		t.Fatal(err)
	}
	var expected Timeline
	if err := json.Unmarshal(read("expected.json"), &expected); err != nil {
		t.Fatal(err)
	}
	return result, expected
}

func assertTimelineJSON(t *testing.T, got, want Timeline) {
	t.Helper()
	gotJSON, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	wantJSON, err := json.Marshal(want)
	if err != nil {
		t.Fatal(err)
	}
	var gotValue, wantValue any
	if err := json.Unmarshal(gotJSON, &gotValue); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(wantJSON, &wantValue); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(gotValue, wantValue) {
		t.Fatalf("timeline mismatch\n got: %s\nwant: %s", gotJSON, wantJSON)
	}
}

func assertConflictNumber(t *testing.T, value ConflictValue, want string) {
	t.Helper()
	if !value.Present {
		t.Fatalf("conflict value is absent, want %s", want)
	}
	number, ok := value.Value.(json.Number)
	if !ok {
		t.Fatalf("conflict value type = %T, want json.Number", value.Value)
	}
	if number.String() != want {
		t.Fatalf("conflict value = %s, want %s", number, want)
	}
}

func cloneTimeline(t *testing.T, timeline Timeline) Timeline {
	t.Helper()
	data, err := json.Marshal(timeline)
	if err != nil {
		t.Fatal(err)
	}
	var cloned Timeline
	if err := json.Unmarshal(data, &cloned); err != nil {
		t.Fatal(err)
	}
	return cloned
}
