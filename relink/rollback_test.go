package relink

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMissingProxyRollsBackWholeTransaction(t *testing.T) {
	dir := t.TempDir()
	root := filepath.Join(dir, "media")
	primaryA := writeMediaFile(t, filepath.Join(root, "a.mp4"), "a", 1000, 0)
	proxyA := writeMediaFile(t, filepath.Join(root, "a.proxy.mp4"), "a-proxy", 1000, 0)
	primaryB := writeMediaFile(t, filepath.Join(root, "b.mp4"), "b", 1000, 0)
	missingProxy := Fingerprint{
		Size:          5,
		DurationMS:    1000,
		FrameRate:     testFrameRate(),
		AudioOffsetMS: 0,
	}
	project := &Project{
		Version:    ProjectVersion,
		FrameRate:  testFrameRate(),
		DurationMS: 2000,
		Clips: []Clip{
			{
				ID:      "clip-a",
				Name:    "a.mp4",
				Primary: MediaRef{Path: `D:\Old\a.mp4`, Fingerprint: primaryA},
				Proxy:   &MediaRef{Path: `D:\Old\a.proxy.mp4`, Fingerprint: proxyA},
			},
			{
				ID:      "clip-b",
				Name:    "b.mp4",
				Primary: MediaRef{Path: `D:\Old\b.mp4`, Fingerprint: primaryB},
				Proxy:   &MediaRef{Path: `D:\Old\b.proxy.mp4`, Fingerprint: missingProxy},
			},
		},
		Timeline: []TimelineItem{
			{ID: "item-a", ClipID: "clip-a", StartFrame: 0, EndFrame: 30},
			{ID: "item-b", ClipID: "clip-b", StartFrame: 30, EndFrame: 60},
		},
	}
	projectPath := filepath.Join(dir, "project.json")
	original := writeProjectFile(t, projectPath, project)
	approver := &recordingApprover{acceptBatch: true}

	_, err := Apply(context.Background(), projectPath, Options{
		Roots:    []string{root},
		Approver: approver,
	})
	if !errors.Is(err, ErrNoCandidates) {
		t.Fatalf("Apply() error = %v, want ErrNoCandidates", err)
	}
	if approver.batchCalls != 0 {
		t.Fatal("approval was requested before all candidates passed validation")
	}
	if got := readFile(t, projectPath); string(got) != string(original) {
		t.Fatal("project changed after missing proxy failure")
	}
}

func TestMissingProxyKeepIsExplicitAndWarned(t *testing.T) {
	dir := t.TempDir()
	root := filepath.Join(dir, "media")
	primaryA := writeMediaFile(t, filepath.Join(root, "a.mp4"), "a", 1000, 0)
	proxyA := writeMediaFile(t, filepath.Join(root, "a.proxy.mp4"), "a-proxy", 1000, 0)
	primaryB := writeMediaFile(t, filepath.Join(root, "b.mp4"), "b", 1000, 0)
	missingProxy := Fingerprint{
		Size:          5,
		DurationMS:    1000,
		FrameRate:     testFrameRate(),
		AudioOffsetMS: 0,
	}
	project := &Project{
		Version:    ProjectVersion,
		FrameRate:  testFrameRate(),
		DurationMS: 2000,
		Clips: []Clip{
			{
				ID:      "clip-a",
				Name:    "a.mp4",
				Primary: MediaRef{Path: `D:\Old\a.mp4`, Fingerprint: primaryA},
				Proxy:   &MediaRef{Path: `D:\Old\a.proxy.mp4`, Fingerprint: proxyA},
			},
			{
				ID:      "clip-b",
				Name:    "b.mp4",
				Primary: MediaRef{Path: `D:\Old\b.mp4`, Fingerprint: primaryB},
				Proxy:   &MediaRef{Path: `D:\Old\b.proxy.mp4`, Fingerprint: missingProxy},
			},
		},
		Timeline: []TimelineItem{
			{ID: "item-a", ClipID: "clip-a", StartFrame: 0, EndFrame: 30},
			{ID: "item-b", ClipID: "clip-b", StartFrame: 30, EndFrame: 60},
		},
	}
	projectPath := filepath.Join(dir, "project.json")
	writeProjectFile(t, projectPath, project)
	approver := &recordingApprover{acceptBatch: true}

	report, err := Apply(context.Background(), projectPath, Options{
		Roots:        []string{root},
		Approver:     approver,
		MissingProxy: MissingProxyKeep,
	})
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	if approver.batchSize != 3 || approver.oneCalls != 0 {
		t.Fatalf("unexpected approvals: %+v", approver)
	}
	if len(report.Warnings) != 1 || report.Warnings[0].Code != "proxy_missing" {
		t.Fatalf("missing proxy warning not reported: %+v", report.Warnings)
	}
	relinked := mustParseProject(t, projectPath)
	if relinked.Clips[1].Proxy.Path != `D:\Old\b.proxy.mp4` {
		t.Fatalf("missing proxy path changed silently: %q", relinked.Clips[1].Proxy.Path)
	}
	if relinked.Clips[0].Primary.Path != filepath.Join(root, "a.mp4") {
		t.Fatalf("valid primary was not relinked: %q", relinked.Clips[0].Primary.Path)
	}
}

func TestInvalidProxyIsNotTreatedAsMissing(t *testing.T) {
	dir := t.TempDir()
	root := filepath.Join(dir, "media")
	primary := writeMediaFile(t, filepath.Join(root, "a.mp4"), "a", 1000, 0)
	expectedProxy := writeMediaFile(t, filepath.Join(root, "a.proxy.mp4"), "a-proxy", 1000, 0)
	expectedProxy.FrameRate = FrameRate{Numerator: 24000, Denominator: 1001}
	project := &Project{
		Version:    ProjectVersion,
		FrameRate:  testFrameRate(),
		DurationMS: 1000,
		Clips: []Clip{{
			ID:      "clip-a",
			Name:    "a.mp4",
			Primary: MediaRef{Path: `D:\Old\a.mp4`, Fingerprint: primary},
			Proxy:   &MediaRef{Path: `D:\Old\a.proxy.mp4`, Fingerprint: expectedProxy},
		}},
		Timeline: []TimelineItem{{ID: "item-a", ClipID: "clip-a", StartFrame: 0, EndFrame: 30}},
	}
	projectPath := filepath.Join(dir, "project.json")
	original := writeProjectFile(t, projectPath, project)

	_, err := Apply(context.Background(), projectPath, Options{
		Roots:        []string{root},
		MissingProxy: MissingProxyKeep,
	})
	if !errors.Is(err, ErrCandidateInvalid) {
		t.Fatalf("Apply() error = %v, want ErrCandidateInvalid", err)
	}
	if got := readFile(t, projectPath); string(got) != string(original) {
		t.Fatal("project changed after invalid proxy failure")
	}
}

func TestAnyMediaMismatchRestoresOriginalProject(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Fingerprint)
	}{
		{
			name: "frame rate",
			mutate: func(fp *Fingerprint) {
				fp.FrameRate = FrameRate{Numerator: 24000, Denominator: 1001}
			},
		},
		{
			name:   "duration",
			mutate: func(fp *Fingerprint) { fp.DurationMS += 100 },
		},
		{
			name:   "audio offset",
			mutate: func(fp *Fingerprint) { fp.AudioOffsetMS += 50 },
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			dir := t.TempDir()
			root := filepath.Join(dir, "media")
			good := writeMediaFile(t, filepath.Join(root, "good.mp4"), "good", 1000, 0)
			bad := writeMediaFile(t, filepath.Join(root, "bad.mp4"), "bad", 1000, 0)
			test.mutate(&bad)
			project := &Project{
				Version:    ProjectVersion,
				FrameRate:  testFrameRate(),
				DurationMS: 2000,
				Clips: []Clip{
					{
						ID:      "clip-good",
						Name:    "good.mp4",
						Primary: MediaRef{Path: `D:\Old\good.mp4`, Fingerprint: good},
					},
					{
						ID:      "clip-bad",
						Name:    "bad.mp4",
						Primary: MediaRef{Path: `D:\Old\bad.mp4`, Fingerprint: bad},
					},
				},
				Timeline: []TimelineItem{
					{ID: "item-good", ClipID: "clip-good", StartFrame: 0, EndFrame: 30},
					{ID: "item-bad", ClipID: "clip-bad", StartFrame: 30, EndFrame: 60},
				},
			}
			projectPath := filepath.Join(dir, "project.json")
			original := writeProjectFile(t, projectPath, project)

			_, err := Apply(context.Background(), projectPath, Options{
				Roots: []string{root},
			})
			if !errors.Is(err, ErrCandidateInvalid) {
				t.Fatalf("Apply() error = %v, want ErrCandidateInvalid", err)
			}
			if got := readFile(t, projectPath); string(got) != string(original) {
				t.Fatal("project changed after media validation failure")
			}
		})
	}
}

type tamperStore struct {
	FileStore
	saves int
}

func (store *tamperStore) Save(path string, data []byte) error {
	store.saves++
	if store.saves == 1 {
		var project Project
		if err := json.Unmarshal(data, &project); err != nil {
			return err
		}
		project.Timeline[0].EndFrame++
		tampered, err := json.MarshalIndent(project, "", "  ")
		if err != nil {
			return err
		}
		data = append(tampered, '\n')
	}
	return store.FileStore.Save(path, data)
}

func TestPostCommitTimelineChangeTriggersRollback(t *testing.T) {
	dir := t.TempDir()
	root := filepath.Join(dir, "media")
	fp := writeMediaFile(t, filepath.Join(root, "a.mp4"), "a", 1000, 0)
	project := &Project{
		Version:    ProjectVersion,
		FrameRate:  testFrameRate(),
		DurationMS: 1000,
		Clips: []Clip{{
			ID:      "clip-a",
			Name:    "a.mp4",
			Primary: MediaRef{Path: `D:\Old\a.mp4`, Fingerprint: fp},
		}},
		Timeline: []TimelineItem{{ID: "item-a", ClipID: "clip-a", StartFrame: 0, EndFrame: 30}},
	}
	projectPath := filepath.Join(dir, "project.json")
	original := writeProjectFile(t, projectPath, project)
	store := &tamperStore{}

	_, err := Apply(context.Background(), projectPath, Options{
		Roots: []string{root},
		Store: store,
	})
	if err == nil || !strings.Contains(err.Error(), "differs from validated plan") {
		t.Fatalf("Apply() error = %v, want committed-plan mismatch", err)
	}
	if store.saves != 2 {
		t.Fatalf("save calls = %d, want initial commit plus rollback", store.saves)
	}
	if got := readFile(t, projectPath); string(got) != string(original) {
		t.Fatal("original project was not restored after post-commit failure")
	}
}

func TestUnknownProjectContentIsRejectedWithoutWrite(t *testing.T) {
	dir := t.TempDir()
	root := filepath.Join(dir, "media")
	fp := writeMediaFile(t, filepath.Join(root, "a.mp4"), "a", 1000, 0)
	project := &Project{
		Version:    ProjectVersion,
		FrameRate:  testFrameRate(),
		DurationMS: 1000,
		Clips: []Clip{{
			ID:      "clip-a",
			Name:    "a.mp4",
			Primary: MediaRef{Path: `D:\Old\a.mp4`, Fingerprint: fp},
		}},
		Timeline: []TimelineItem{{ID: "item-a", ClipID: "clip-a", StartFrame: 0, EndFrame: 30}},
	}
	projectPath := filepath.Join(dir, "project.json")
	raw := writeProjectFile(t, projectPath, project)
	withExtra := strings.Replace(string(raw), "{\n", "{\n  \"markers\": [1, 2],\n", 1)
	if err := os.WriteFile(projectPath, []byte(withExtra), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := Apply(context.Background(), projectPath, Options{
		Roots: []string{root},
	})
	if err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("Apply() error = %v, want unknown-field rejection", err)
	}
	if got := readFile(t, projectPath); string(got) != withExtra {
		t.Fatal("project with unknown content was rewritten")
	}
}
