package relink

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func testFrameRate() FrameRate {
	return FrameRate{Numerator: 30000, Denominator: 1001}
}

func writeMediaFile(
	t *testing.T,
	path string,
	data string,
	durationMS int64,
	audioOffsetMS int64,
) Fingerprint {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
	sidecar := timingSidecar{
		DurationMS:    durationMS,
		FrameRate:     testFrameRate(),
		AudioOffsetMS: audioOffsetMS,
	}
	sidecarBytes, err := json.Marshal(sidecar)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path+MetadataSuffix, sidecarBytes, 0o644); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256([]byte(data))
	return Fingerprint{
		Size:          int64(len(data)),
		SHA256:        fmt.Sprintf("%x", sum),
		DurationMS:    durationMS,
		FrameRate:     testFrameRate(),
		AudioOffsetMS: audioOffsetMS,
	}
}

func writeProjectFile(t *testing.T, path string, project *Project) []byte {
	t.Helper()
	data, err := json.MarshalIndent(project, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	return data
}

func readFile(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func mustParseProject(t *testing.T, path string) *Project {
	t.Helper()
	data := readFile(t, path)
	project, err := ParseProject(data)
	if err != nil {
		t.Fatal(err)
	}
	return project
}

type recordingApprover struct {
	batchCalls  int
	oneCalls    int
	batchSize   int
	acceptBatch bool
	choose      func(Proposal) Approval
}

func (approver *recordingApprover) ApproveBatch(_ context.Context, proposals []Proposal) (bool, error) {
	approver.batchCalls++
	approver.batchSize = len(proposals)
	return approver.acceptBatch, nil
}

func (approver *recordingApprover) ApproveOne(_ context.Context, proposal Proposal) (Approval, error) {
	approver.oneCalls++
	if approver.choose == nil {
		return Approval{}, nil
	}
	return approver.choose(proposal), nil
}
