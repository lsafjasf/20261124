package project

import (
	"os"
	"path/filepath"
	"testing"
)

func writeFile(t *testing.T, name, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadFlattensNestedTracks(t *testing.T) {
	content := `{
	  "schemaVersion": "1.0",
	  "name": "p",
	  "rate": 25,
	  "media": [{"id": "m1", "name": "a", "path": "/a", "checksum": "h1"}],
	  "tracks": [{
	    "id": "V1", "kind": "group",
	    "children": [{"id": "A1", "kind": "video", "clips": [
	      {"id": "c1", "mediaId": "m1", "timelineIn": 0, "sourceIn": 1, "sourceOut": 2}
	    ]}]
	  }]
	}`
	l, err := LoadFile(writeFile(t, "p.json", content))
	if err != nil {
		t.Fatal(err)
	}
	if len(l.Tracks) != 2 || l.Tracks[1].Path[0] != "V1" || l.Tracks[1].Path[1] != "A1" {
		t.Fatalf("track flatten wrong: %+v", l.Tracks)
	}
	if len(l.Clips) != 1 || pathJoin(l.Clips[0].TrackPath) != "V1/A1" {
		t.Fatalf("clip flatten wrong: %+v", l.Clips)
	}
	if l.Clips[0].Clip.Speed != 1 {
		t.Fatalf("speed should default to 1, got %v", l.Clips[0].Clip.Speed)
	}
	if l.SHA256 == "" {
		t.Fatal("sha256 fingerprint missing")
	}
}

func TestValidationIssues(t *testing.T) {
	content := `{
	  "schemaVersion": "1.0",
	  "name": "p",
	  "rate": 25,
	  "media": [{"id": "m1", "name": "a", "path": "/a", "checksum": "h1"}],
	  "tracks": [
	    {"id": "V1", "clips": [
	      {"id": "dup", "mediaId": "m1", "timelineIn": 0, "sourceIn": 5, "sourceOut": 1},
	      {"id": "dup", "mediaId": "m1", "timelineIn": 2, "sourceIn": 0, "sourceOut": 1},
	      {"id": "ghost", "mediaId": "nope", "timelineIn": 3, "sourceIn": 0, "sourceOut": 1}
	    ]},
	    {"id": "V1", "clips": []}
	  ]
	}`
	l, err := LoadFile(writeFile(t, "p.json", content))
	if err != nil {
		t.Fatal(err)
	}
	kinds := map[string]bool{}
	for _, is := range l.Issues {
		kinds[is.Kind] = true
	}
	for _, want := range []string{"CLIP_DUPLICATE_ID", "CLIP_UNKNOWN_MEDIA", "CLIP_BAD_SOURCE_RANGE", "TRACK_DUPLICATE_ID"} {
		if !kinds[want] {
			t.Fatalf("missing issue %s, got %+v", want, l.Issues)
		}
	}
}

func TestRejectsUnknownFields(t *testing.T) {
	content := `{"schemaVersion":"1.0","bogus":1,"tracks":[]}`
	if _, err := LoadFile(writeFile(t, "p.json", content)); err == nil {
		t.Fatal("expected parse error for unknown field")
	}
}

func pathJoin(p []string) string {
	out := ""
	for i, s := range p {
		if i > 0 {
			out += "/"
		}
		out += s
	}
	return out
}
