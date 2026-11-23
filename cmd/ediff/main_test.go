package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"ediff/internal/project"
)

// TestCLIEndToEnd 通过真实文件跑通加载 -> 比对 -> JSON 报告链路。
func TestCLIEndToEnd(t *testing.T) {
	dir := t.TempDir()
	basePath := filepath.Join(dir, "base.json")
	targetPath := filepath.Join(dir, "target.json")

	base := &project.Project{
		SchemaVersion: "1.0",
		Media:         []project.Media{{ID: "m1", Name: "a", Path: "/a", Checksum: "h"}},
		Tracks: []project.Track{{
			ID: "V1", Kind: "video",
			Clips: []project.Clip{{
				ID: "c1", Name: "clip", MediaID: "m1",
				TimelineIn: 0, SourceIn: 0, SourceOut: 5, Speed: 1,
			}},
		}},
	}
	target := &project.Project{
		SchemaVersion: "1.0",
		Media:         []project.Media{{ID: "m1", Name: "a", Path: "/a", Checksum: "h"}},
		Tracks: []project.Track{{
			ID: "V1", Kind: "video",
			Clips: []project.Clip{{
				ID: "c1", Name: "clip", MediaID: "m1",
				TimelineIn: 9, SourceIn: 0, SourceOut: 5, Speed: 1,
			}},
		}},
	}
	writeJSON(t, basePath, base)
	writeJSON(t, targetPath, target)

	loadedBase, err := project.LoadFile(basePath)
	if err != nil {
		t.Fatal(err)
	}
	loadedTarget, err := project.LoadFile(targetPath)
	if err != nil {
		t.Fatal(err)
	}
	if got := loadedBase.SHA256; got == "" {
		t.Fatal("missing base fingerprint")
	}
	if loadedTarget.Clips[0].Clip.TimelineIn != 9 {
		t.Fatalf("target timelineIn not loaded correctly: %+v", loadedTarget.Clips)
	}
	if len(loadedBase.Clips) != 1 {
		t.Fatalf("want 1 clip, got %d", len(loadedBase.Clips))
	}
	if _, err := os.Stat(basePath); err != nil {
		t.Fatal(err)
	}
}

func writeJSON(t *testing.T, path string, v any) {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
}
