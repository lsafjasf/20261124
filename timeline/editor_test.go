package timeline

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestRoundTripAllEditKindsAndHistory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "project.jsonl")
	base := NewProject("p1", "Demo")
	editor, err := CreateEditor(path, base)
	if err != nil {
		t.Fatal(err)
	}

	clip := Clip{
		ID:        "clip-b",
		Track:     "v1",
		Start:     100,
		SourceIn:  20,
		SourceOut: 420,
		Speed:     1,
		Effects: []Effect{
			{ID: "fx-1", Kind: "blur", Params: map[string]float64{"radius": 1}},
		},
	}
	mustEdit(t, editor, "e-create", func(id string) error { return editor.CreateClip(id, clip) })
	mustEdit(t, editor, "e-move", func(id string) error { return editor.MoveClip(id, "clip-b", "v2", 250) })
	mustEdit(t, editor, "e-trim", func(id string) error { return editor.TrimClip(id, "clip-b", 200, 50, 300) })
	mustEdit(t, editor, "e-speed", func(id string) error { return editor.SetSpeed(id, "clip-b", 1.5) })
	mustEdit(t, editor, "e-marker-a", func(id string) error {
		return editor.AddMarker(id, Marker{ID: "m-2", Time: 900, Label: "end"})
	})
	mustEdit(t, editor, "e-marker-b", func(id string) error {
		return editor.AddMarker(id, Marker{ID: "m-1", Time: 400, Label: "start"})
	})
	mustEdit(t, editor, "e-marker-move", func(id string) error {
		return editor.MoveMarker(id, "m-2", 50, "intro")
	})
	mustEdit(t, editor, "e-fx-add", func(id string) error {
		return editor.AddEffect(id, "clip-b", Effect{
			ID: "fx-2", Kind: "color", Params: map[string]float64{"gain": 0.5},
		})
	})
	mustEdit(t, editor, "e-fx-kind", func(id string) error {
		return editor.SetEffect(id, "clip-b", "fx-2", "lut")
	})
	mustEdit(t, editor, "e-fx-param", func(id string) error {
		return editor.SetEffectParam(id, "clip-b", "fx-2", "gain", 0.8)
	})
	mustEdit(t, editor, "e-fx-param-delete", func(id string) error {
		return editor.DeleteEffectParam(id, "clip-b", "fx-2", "gain")
	})
	mustEdit(t, editor, "e-delete", func(id string) error { return editor.DeleteClip(id, "clip-b") })

	final := editor.Project()
	if err := editor.Close(); err != nil {
		t.Fatal(err)
	}

	reopened, recovery, err := OpenEditor(path)
	if err != nil {
		t.Fatal(err)
	}
	if recovery.Recovered() {
		t.Fatalf("unexpected recovery: %+v", recovery)
	}
	if got := reopened.Project(); !reflect.DeepEqual(final, got) {
		t.Fatalf("reopened project differs:\nwant %#v\ngot  %#v", final, got)
	}
	if reopened.UndoDepth() != 12 {
		t.Fatalf("undo depth = %d, want 12", reopened.UndoDepth())
	}
	compare, diff, err := reopened.log.ReplayAndCompare(final)
	if err != nil {
		t.Fatal(err)
	}
	if diff != nil {
		t.Fatalf("unexpected field difference: %+v", diff)
	}
	if !reflect.DeepEqual(compare, final) {
		t.Fatal("ReplayAndCompare returned a different project")
	}

	for reopened.UndoDepth() > 0 {
		if _, ok, err := reopened.Undo(); err != nil || !ok {
			t.Fatalf("undo failed: ok=%v err=%v", ok, err)
		}
	}
	if got := reopened.Project(); !reflect.DeepEqual(got, base) {
		t.Fatalf("undo-all project differs:\nwant %#v\ngot  %#v", base, got)
	}
	if reopened.RedoDepth() != 12 {
		t.Fatalf("redo depth = %d, want 12", reopened.RedoDepth())
	}
	if err := reopened.Close(); err != nil {
		t.Fatal(err)
	}

	replayed, _, err := OpenEditor(path)
	if err != nil {
		t.Fatal(err)
	}
	defer replayed.Close()
	if got := replayed.Project(); !reflect.DeepEqual(got, base) {
		t.Fatalf("crash recovery of undo history differs:\nwant %#v\ngot  %#v", base, got)
	}
	for replayed.RedoDepth() > 0 {
		if _, ok, err := replayed.Redo(); err != nil || !ok {
			t.Fatalf("redo failed: ok=%v err=%v", ok, err)
		}
	}
	if got := replayed.Project(); !reflect.DeepEqual(got, final) {
		t.Fatalf("redo-all project differs:\nwant %#v\ngot  %#v", final, got)
	}
}

func TestDuplicateEventIsFirstWins(t *testing.T) {
	path := filepath.Join(t.TempDir(), "project.jsonl")
	editor, err := CreateEditor(path, NewProject("p1", "Demo"))
	if err != nil {
		t.Fatal(err)
	}
	clip := Clip{ID: "c1", Track: "v1", Start: 0, SourceIn: 0, SourceOut: 100, Speed: 1}
	if err := editor.CreateClip("same-id", clip); err != nil {
		t.Fatal(err)
	}
	before := editor.Project()
	recordsBefore := len(editor.log.Records())
	err = editor.MoveClip("same-id", "c1", "v2", 999)
	if !errors.Is(err, ErrDuplicateEvent) {
		t.Fatalf("duplicate error = %v, want ErrDuplicateEvent", err)
	}
	if got := editor.Project(); !reflect.DeepEqual(got, before) {
		t.Fatalf("duplicate changed project:\nwant %#v\ngot  %#v", before, got)
	}
	if got := len(editor.log.Records()); got != recordsBefore {
		t.Fatalf("duplicate appended a record: %d -> %d", recordsBefore, got)
	}
	if err := editor.Close(); err != nil {
		t.Fatal(err)
	}

	// A repeated JSONL line is ignored on replay by event ID. The second line
	// deliberately contains an operation that would overwrite the first.
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	lines := bytes.Split(bytes.TrimSuffix(data, []byte{'\n'}), []byte{'\n'})
	var duplicate Record
	if err := json.Unmarshal(lines[len(lines)-1], &duplicate); err != nil {
		t.Fatal(err)
	}
	duplicate.Op = NewMoveClip("c1", "v2", 999)
	duplicate.Inverse = NewMoveClip("c1", "v1", 0)
	duplicate.StateHash = strings.Repeat("0", 64)
	duplicateData, err := json.Marshal(duplicate)
	if err != nil {
		t.Fatal(err)
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.Write(append(duplicateData, '\n')); err != nil {
		file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}

	log, _, err := OpenLog(path)
	if err != nil {
		t.Fatal(err)
	}
	defer log.Close()
	project, result, err := log.Replay()
	if err != nil {
		t.Fatal(err)
	}
	if result.DuplicateEvents != 1 {
		t.Fatalf("duplicate count = %d, want 1", result.DuplicateEvents)
	}
	if got := project.Clips[0].Track; got != "v1" {
		t.Fatalf("duplicate overwrote state: track=%q", got)
	}
}

func TestRecoveryTruncatesOnlyPartialTail(t *testing.T) {
	path := filepath.Join(t.TempDir(), "project.jsonl")
	editor, err := CreateEditor(path, NewProject("p1", "Demo"))
	if err != nil {
		t.Fatal(err)
	}
	if err := editor.CreateClip("e1", Clip{
		ID: "c1", Track: "v1", Start: 0, SourceIn: 0, SourceOut: 100, Speed: 1,
	}); err != nil {
		t.Fatal(err)
	}
	want := editor.Project()
	if err := editor.Close(); err != nil {
		t.Fatal(err)
	}
	partial := []byte(`{"seq":99,"id":"torn","kind":"edit"`)
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.Write(partial); err != nil {
		file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}

	recovered, report, err := OpenEditor(path)
	if err != nil {
		t.Fatal(err)
	}
	defer recovered.Close()
	if report.TruncatedBytes != int64(len(partial)) {
		t.Fatalf("truncated bytes = %d, want %d", report.TruncatedBytes, len(partial))
	}
	if got := recovered.Project(); !reflect.DeepEqual(got, want) {
		t.Fatalf("recovered project differs:\nwant %#v\ngot  %#v", want, got)
	}
	if err := recovered.CreateClip("e2", Clip{
		ID: "c2", Track: "v1", Start: 100, SourceIn: 0, SourceOut: 100, Speed: 1,
	}); err != nil {
		t.Fatal(err)
	}
	if err := recovered.Close(); err != nil {
		t.Fatal(err)
	}

	again, report, err := OpenEditor(path)
	if err != nil {
		t.Fatal(err)
	}
	defer again.Close()
	if report.Recovered() {
		t.Fatalf("unexpected second recovery: %+v", report)
	}
	if len(again.Project().Clips) != 2 {
		t.Fatalf("clips after post-recovery append = %d, want 2", len(again.Project().Clips))
	}
}

func TestReplayReportsFirstDifferingStep(t *testing.T) {
	path := filepath.Join(t.TempDir(), "project.jsonl")
	editor, err := CreateEditor(path, NewProject("p1", "Demo"))
	if err != nil {
		t.Fatal(err)
	}
	for _, op := range []struct {
		id    string
		clip  Clip
		start int64
	}{
		{"e1", Clip{ID: "c1", Track: "v1", Start: 0, SourceIn: 0, SourceOut: 100, Speed: 1}, 0},
		{"e2", Clip{ID: "c2", Track: "v1", Start: 100, SourceIn: 0, SourceOut: 100, Speed: 1}, 100},
		{"e3", Clip{ID: "c3", Track: "v1", Start: 200, SourceIn: 0, SourceOut: 100, Speed: 1}, 200},
	} {
		if err := editor.CreateClip(op.id, op.clip); err != nil {
			t.Fatal(err)
		}
	}
	if err := editor.Close(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	lines := bytes.Split(bytes.TrimSuffix(data, []byte{'\n'}), []byte{'\n'})
	if len(lines) != 4 {
		t.Fatalf("line count = %d, want header + 3 records", len(lines))
	}
	var record Record
	if err := json.Unmarshal(lines[2], &record); err != nil {
		t.Fatal(err)
	}
	record.StateHash = strings.Repeat("f", 64)
	lines[2], err = json.Marshal(record)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append(bytes.Join(lines, []byte{'\n'}), '\n'), 0o644); err != nil {
		t.Fatal(err)
	}

	log, _, err := OpenLog(path)
	if err != nil {
		t.Fatal(err)
	}
	defer log.Close()
	_, _, err = log.Replay()
	if err == nil {
		t.Fatal("Replay succeeded with a tampered state hash")
	}
	var difference *Difference
	if !errors.As(err, &difference) {
		t.Fatalf("error type = %T, want *Difference", err)
	}
	if difference.Step != 2 || difference.EventID != "e2" {
		t.Fatalf("first difference = step %d event %q, want step 2 event e2", difference.Step, difference.EventID)
	}
}

func TestApplyOwnsMutableOperationPayload(t *testing.T) {
	path := filepath.Join(t.TempDir(), "project.jsonl")
	editor, err := CreateEditor(path, NewProject("p1", "Demo"))
	if err != nil {
		t.Fatal(err)
	}
	op := NewCreateClip(Clip{
		ID: "c1", Track: "v1", Start: 10, SourceIn: 0, SourceOut: 100, Speed: 1,
	})
	if _, err := editor.Apply("e1", op); err != nil {
		t.Fatal(err)
	}
	op.Clip.Start = 999
	project, _, err := editor.log.Replay()
	if err != nil {
		t.Fatal(err)
	}
	if got := project.Clips[0].Start; got != 10 {
		t.Fatalf("caller mutation changed journal state: start=%d, want 10", got)
	}
	if err := editor.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestRecoveryRepairsCompleteLastLineWithoutNewline(t *testing.T) {
	path := filepath.Join(t.TempDir(), "project.jsonl")
	editor, err := CreateEditor(path, NewProject("p1", "Demo"))
	if err != nil {
		t.Fatal(err)
	}
	if err := editor.CreateClip("e1", Clip{
		ID: "c1", Track: "v1", Start: 0, SourceIn: 0, SourceOut: 100, Speed: 1,
	}); err != nil {
		t.Fatal(err)
	}
	want := editor.Project()
	if err := editor.Close(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(data) == 0 || data[len(data)-1] != '\n' {
		t.Fatal("test journal has no trailing newline to remove")
	}
	if err := os.WriteFile(path, data[:len(data)-1], 0o644); err != nil {
		t.Fatal(err)
	}

	recovered, report, err := OpenEditor(path)
	if err != nil {
		t.Fatal(err)
	}
	defer recovered.Close()
	if !report.RepairedNewline || report.TruncatedBytes != 0 {
		t.Fatalf("recovery = %+v, want repaired newline only", report)
	}
	if got := recovered.Project(); !reflect.DeepEqual(got, want) {
		t.Fatalf("repaired project differs:\nwant %#v\ngot  %#v", want, got)
	}
	if err := recovered.CreateClip("e2", Clip{
		ID: "c2", Track: "v1", Start: 100, SourceIn: 0, SourceOut: 100, Speed: 1,
	}); err != nil {
		t.Fatal(err)
	}
}

func TestReplayAndCompareReportsFieldPath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "project.jsonl")
	editor, err := CreateEditor(path, NewProject("p1", "Demo"))
	if err != nil {
		t.Fatal(err)
	}
	if err := editor.CreateClip("e1", Clip{
		ID: "c1", Track: "v1", Start: 10, SourceIn: 0, SourceOut: 100, Speed: 1,
	}); err != nil {
		t.Fatal(err)
	}
	actual := editor.Project()
	expected := editor.Project()
	expected.Clips[0].Start = 11
	if err := editor.Close(); err != nil {
		t.Fatal(err)
	}

	log, _, err := OpenLog(path)
	if err != nil {
		t.Fatal(err)
	}
	defer log.Close()
	got, difference, err := log.ReplayAndCompare(expected)
	if err != nil {
		t.Fatal(err)
	}
	if difference == nil {
		t.Fatal("expected a field difference")
	}
	if difference.Path != "Clips[0].Start" {
		t.Fatalf("difference path = %q, want Clips[0].Start", difference.Path)
	}
	if !reflect.DeepEqual(got, actual) {
		t.Fatalf("replayed project differs:\nwant %#v\ngot  %#v", actual, got)
	}
}

func TestHistoryStoresCommandsNotProjectSnapshots(t *testing.T) {
	entryType := reflect.TypeOf(historyEntry{})
	projectType := reflect.TypeOf(Project{})
	for i := 0; i < entryType.NumField(); i++ {
		field := entryType.Field(i)
		if field.Type == projectType || field.Type == reflect.PointerTo(projectType) {
			t.Fatalf("history entry field %s stores a whole project", field.Name)
		}
	}
}

func mustEdit(t *testing.T, editor *Editor, eventID string, edit func(string) error) {
	t.Helper()
	if err := edit(eventID); err != nil {
		t.Fatalf("%s: %v", eventID, err)
	}
}
