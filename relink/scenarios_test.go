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

func TestDriveLetterChangeBatchRelinksAndPreservesTimeline(t *testing.T) {
	dir := t.TempDir()
	root := filepath.Join(dir, "relinked")
	primaryPath := filepath.Join(root, "Footage", "ep1", "cam-a.mp4")
	proxyPath := filepath.Join(root, "Proxies", "ep1", "cam-a.proxy.mp4")
	primaryFP := writeMediaFile(t, primaryPath, "camera-a-original", 4200, 7)
	proxyFP := writeMediaFile(t, proxyPath, "camera-a-proxy", 4200, 7)

	project := &Project{
		Version:       ProjectVersion,
		Name:          "drive-change",
		FrameRate:     testFrameRate(),
		DurationMS:    4200,
		AudioOffsetMS: 7,
		Clips: []Clip{{
			ID:   "clip-a",
			Name: "cam-a.mp4",
			Primary: MediaRef{
				Path:        `D:\Media\Footage\ep1\cam-a.mp4`,
				Fingerprint: primaryFP,
			},
			Proxy: &MediaRef{
				Path:        `D:\Media\Proxies\ep1\cam-a.proxy.mp4`,
				Fingerprint: proxyFP,
			},
		}},
		Timeline: []TimelineItem{{
			ID:            "item-1",
			ClipID:        "clip-a",
			StartFrame:    0,
			EndFrame:      120,
			SourceInFrame: 24,
			AudioOffsetMS: 7,
		}},
	}
	projectPath := filepath.Join(dir, "project.json")
	writeProjectFile(t, projectPath, project)
	beforeTimeline, _ := json.Marshal(project.Timeline)

	report, err := Apply(context.Background(), projectPath, Options{
		Roots: []string{root},
	})
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	if report.BatchAccepted != 2 || report.LowAccepted != 0 {
		t.Fatalf("unexpected approvals: %+v", report)
	}
	if len(report.Links) != 2 || len(report.Checks) != 2 {
		t.Fatalf("unexpected report: %+v", report)
	}
	for _, check := range report.Checks {
		if !check.FrameRateOK || !check.DurationOK || !check.AudioOffsetOK {
			t.Fatalf("media check failed: %+v", check)
		}
	}

	relinked := mustParseProject(t, projectPath)
	if relinked.Clips[0].Primary.Path != primaryPath {
		t.Errorf("primary path = %q, want %q", relinked.Clips[0].Primary.Path, primaryPath)
	}
	if relinked.Clips[0].Proxy.Path != proxyPath {
		t.Errorf("proxy path = %q, want %q", relinked.Clips[0].Proxy.Path, proxyPath)
	}
	afterTimeline, _ := json.Marshal(relinked.Timeline)
	if string(beforeTimeline) != string(afterTimeline) {
		t.Fatalf("timeline changed: before=%s after=%s", beforeTimeline, afterTimeline)
	}
	if relinked.FrameRate != project.FrameRate ||
		relinked.DurationMS != project.DurationMS ||
		relinked.AudioOffsetMS != project.AudioOffsetMS {
		t.Fatal("project timing fields changed")
	}
}

func TestDuplicateFilesRequireOneConfirmationPerItem(t *testing.T) {
	dir := t.TempDir()
	rootA := filepath.Join(dir, "copy-a")
	rootB := filepath.Join(dir, "copy-b")
	fpA := writeMediaFile(t, filepath.Join(rootA, "a.mp4"), "duplicate-a", 1000, 3)
	fpB := writeMediaFile(t, filepath.Join(rootA, "b.mp4"), "duplicate-b", 2000, 4)
	writeMediaFile(t, filepath.Join(rootB, "a.mp4"), "duplicate-a", 1000, 3)
	writeMediaFile(t, filepath.Join(rootB, "b.mp4"), "duplicate-b", 2000, 4)

	project := &Project{
		Version:       ProjectVersion,
		Name:          "duplicates",
		FrameRate:     testFrameRate(),
		DurationMS:    3000,
		AudioOffsetMS: 3,
		Clips: []Clip{
			{
				ID:   "clip-a",
				Name: "a.mp4",
				Primary: MediaRef{
					Path:        `D:\Missing\a.mp4`,
					Fingerprint: fpA,
				},
			},
			{
				ID:   "clip-b",
				Name: "b.mp4",
				Primary: MediaRef{
					Path:        `D:\Missing\b.mp4`,
					Fingerprint: fpB,
				},
			},
		},
		Timeline: []TimelineItem{
			{ID: "item-a", ClipID: "clip-a", StartFrame: 0, EndFrame: 30, AudioOffsetMS: 3},
			{ID: "item-b", ClipID: "clip-b", StartFrame: 30, EndFrame: 90, AudioOffsetMS: 4},
		},
	}
	projectPath := filepath.Join(dir, "project.json")
	writeProjectFile(t, projectPath, project)

	approver := &recordingApprover{
		acceptBatch: true,
		choose: func(proposal Proposal) Approval {
			for _, candidate := range proposal.Alternatives {
				if strings.HasPrefix(candidate.Path, rootB+string(os.PathSeparator)) {
					return Approval{Accept: true, CandidatePath: candidate.Path}
				}
			}
			return Approval{}
		},
	}
	report, err := Apply(context.Background(), projectPath, Options{
		Roots:    []string{rootA, rootB},
		Approver: approver,
	})
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	if approver.batchCalls != 0 || approver.oneCalls != 2 {
		t.Fatalf("batch calls=%d one calls=%d, want 0 and 2",
			approver.batchCalls, approver.oneCalls)
	}
	if report.BatchAccepted != 0 || report.LowAccepted != 2 {
		t.Fatalf("unexpected approvals: %+v", report)
	}
	relinked := mustParseProject(t, projectPath)
	if !strings.HasPrefix(relinked.Clips[0].Primary.Path, rootB+string(os.PathSeparator)) {
		t.Errorf("clip-a was not relinked to selected duplicate: %s", relinked.Clips[0].Primary.Path)
	}
	if !strings.HasPrefix(relinked.Clips[1].Primary.Path, rootB+string(os.PathSeparator)) {
		t.Errorf("clip-b was not relinked to selected duplicate: %s", relinked.Clips[1].Primary.Path)
	}
}

func TestDuplicateFileRejectionLeavesProjectUntouched(t *testing.T) {
	dir := t.TempDir()
	rootA := filepath.Join(dir, "copy-a")
	rootB := filepath.Join(dir, "copy-b")
	fp := writeMediaFile(t, filepath.Join(rootA, "a.mp4"), "duplicate", 1000, 0)
	writeMediaFile(t, filepath.Join(rootB, "a.mp4"), "duplicate", 1000, 0)
	project := &Project{
		Version:    ProjectVersion,
		FrameRate:  testFrameRate(),
		DurationMS: 1000,
		Clips: []Clip{{
			ID:      "clip-a",
			Name:    "a.mp4",
			Primary: MediaRef{Path: `D:\Missing\a.mp4`, Fingerprint: fp},
		}},
		Timeline: []TimelineItem{{ID: "item-a", ClipID: "clip-a", StartFrame: 0, EndFrame: 30}},
	}
	projectPath := filepath.Join(dir, "project.json")
	original := writeProjectFile(t, projectPath, project)

	_, err := Apply(context.Background(), projectPath, Options{
		Roots: []string{rootA, rootB},
	})
	if !errors.Is(err, ErrNotConfirmed) {
		t.Fatalf("Apply() error = %v, want ErrNotConfirmed", err)
	}
	if got := readFile(t, projectPath); string(got) != string(original) {
		t.Fatal("project changed after low-confidence rejection")
	}
}

func TestBatchAndIndividualConfirmationsAreSeparated(t *testing.T) {
	dir := t.TempDir()
	uniqueRoot := filepath.Join(dir, "unique")
	duplicateRootA := filepath.Join(dir, "duplicate-a")
	duplicateRootB := filepath.Join(dir, "duplicate-b")
	uniqueFP := writeMediaFile(t, filepath.Join(uniqueRoot, "unique.mp4"), "unique", 1000, 0)
	duplicateFP := writeMediaFile(t, filepath.Join(duplicateRootA, "dup.mp4"), "dup", 2000, 0)
	writeMediaFile(t, filepath.Join(duplicateRootB, "dup.mp4"), "dup", 2000, 0)
	project := &Project{
		Version:    ProjectVersion,
		FrameRate:  testFrameRate(),
		DurationMS: 3000,
		Clips: []Clip{
			{
				ID:      "clip-unique",
				Name:    "unique.mp4",
				Primary: MediaRef{Path: `D:\Old\unique.mp4`, Fingerprint: uniqueFP},
			},
			{
				ID:      "clip-dup",
				Name:    "dup.mp4",
				Primary: MediaRef{Path: `D:\Old\dup.mp4`, Fingerprint: duplicateFP},
			},
		},
		Timeline: []TimelineItem{
			{ID: "item-unique", ClipID: "clip-unique", StartFrame: 0, EndFrame: 30},
			{ID: "item-dup", ClipID: "clip-dup", StartFrame: 30, EndFrame: 90},
		},
	}
	projectPath := filepath.Join(dir, "project.json")
	writeProjectFile(t, projectPath, project)
	approver := &recordingApprover{
		acceptBatch: true,
		choose: func(Proposal) Approval {
			return Approval{Accept: true}
		},
	}

	report, err := Apply(context.Background(), projectPath, Options{
		Roots:    []string{uniqueRoot, duplicateRootA, duplicateRootB},
		Approver: approver,
	})
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	if approver.batchCalls != 1 || approver.batchSize != 1 || approver.oneCalls != 1 {
		t.Fatalf("unexpected confirmation calls: %+v", approver)
	}
	if report.BatchAccepted != 1 || report.LowAccepted != 1 {
		t.Fatalf("unexpected report counts: %+v", report)
	}
}
