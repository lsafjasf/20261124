package review

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadConfigResolvesSourcesAndInheritsRegion(t *testing.T) {
	dir := t.TempDir()
	config := `{
	  "fps": 25,
	  "tracks": [
	    {
	      "id": "f",
	      "kind": "forced",
	      "source": "forced.srt",
	      "region": {"x": 0.1, "y": 0.7, "width": 0.8, "height": 0.1}
	    },
	    {
	      "id": "u",
	      "kind": "full-subtitle",
	      "source": "full.srt",
	      "region": {"x": 0.1, "y": 0.7, "width": 0.8, "height": 0.1}
	    }
	  ]
	}`
	forced := "1\n00:00:01,000 --> 00:00:02,000\n注意安全\n"
	full := "1\n00:00:01,000 --> 00:00:02,000\n注意安全\n"
	writeTestFile(t, filepath.Join(dir, "review.json"), config)
	writeTestFile(t, filepath.Join(dir, "forced.srt"), forced)
	writeTestFile(t, filepath.Join(dir, "full.srt"), full)

	cfg, err := LoadConfig(filepath.Join(dir, "review.json"))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Tracks[0].Kind != "forced_narrative" || cfg.Tracks[1].Kind != "full" {
		t.Fatalf("kinds were not canonicalized: %+v", cfg.Tracks)
	}
	if cfg.Tracks[0].Cues[0].Region == nil {
		t.Fatal("track default region was not inherited")
	}
	report, err := Analyze(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if report.ConflictCount != 1 {
		t.Fatalf("got %d conflicts, want 1", report.ConflictCount)
	}
}

func TestLoadConfigRejectsUnknownField(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "review.json")
	writeTestFile(t, path, `{"fps":25,"unexpected":true,"tracks":[]}`)
	if _, err := LoadConfig(path); err == nil {
		t.Fatal("expected unknown field error")
	}
}

func writeTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}
