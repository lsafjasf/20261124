package diff

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"ediff/internal/project"
)

func mustWriteProject(t *testing.T, path string, p *project.Project) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
}

// mustLoad 经临时文件走完整 LoadFile 路径（含校验与展开）。
func mustLoad(t *testing.T, p *project.Project) *project.Loaded {
	t.Helper()
	path := filepath.Join(t.TempDir(), "p.json")
	mustWriteProject(t, path, p)
	l, err := project.LoadFile(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	return l
}

func sampleProject() *project.Project {
	return &project.Project{
		SchemaVersion: "1.0",
		Name:          "sample",
		Rate:          25,
		Media: []project.Media{
			{ID: "m1", Name: "a.mov", Path: "/data/a.mov", Checksum: "hash-a", Duration: 100},
			{ID: "m2", Name: "b.mov", Path: "/data/b.mov", Checksum: "hash-b", Duration: 100},
		},
		Tracks: []project.Track{{
			ID:   "V1",
			Name: "Video 1",
			Kind: "video",
			Clips: []project.Clip{
				{
					ID: "c1", Name: "intro", MediaID: "m1", TrackID: "V1",
					TimelineIn: 0, SourceIn: 0, SourceOut: 10, Speed: 1,
					Enabled: boolp(true),
					Transition: &project.Transition{
						ID: "tr1", Type: "dissolve", At: "head", Duration: 1,
						Params: map[string]float64{"softness": 0.5},
					},
					Effects: []project.Effect{{
						ID: "fx1", Type: "blur", Enabled: boolp(true),
						Params: map[string]float64{"amount": 10},
					}},
				},
				{
					ID: "c2", Name: "outro", MediaID: "m2", TrackID: "V1",
					TimelineIn: 10, SourceIn: 0, SourceOut: 5, Speed: 1,
					Enabled: boolp(true),
				},
			},
		}},
	}
}

func nestedProject() *project.Project {
	c1 := project.Clip{
		ID: "c1", Name: "n1", MediaID: "m1",
		TimelineIn: 0, SourceIn: 0, SourceOut: 10, Speed: 1, Enabled: boolp(true),
	}
	c2 := project.Clip{
		ID: "c2", Name: "n2", MediaID: "m2",
		TimelineIn: 10, SourceIn: 0, SourceOut: 5, Speed: 1, Enabled: boolp(true),
	}
	return &project.Project{
		SchemaVersion: "1.0",
		Name:          "nested",
		Rate:          25,
		Media: []project.Media{
			{ID: "m1", Name: "a.mov", Path: "/data/a.mov", Checksum: "hash-a"},
			{ID: "m2", Name: "b.mov", Path: "/data/b.mov", Checksum: "hash-b"},
		},
		Tracks: []project.Track{{
			ID:   "V1",
			Name: "container",
			Kind: "group",
			Children: []project.Track{
				{ID: "A1", Name: "inner-1", Kind: "video", Clips: []project.Clip{c1}},
				{ID: "A2", Name: "inner-2", Kind: "video", Clips: []project.Clip{c2}},
			},
		}},
	}
}
