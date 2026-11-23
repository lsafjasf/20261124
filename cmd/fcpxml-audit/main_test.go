package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLossyAuditReturnsNonZero(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{
		"audit",
		"-source", filepath.Join("..", "..", "testdata", "source.fcpxml"),
		"-candidate", filepath.Join("..", "..", "testdata", "lossy.fcpxml"),
	}, &stdout, &stderr)
	if code != 2 {
		t.Fatalf("exit = %d, want 2; stderr=%s", code, stderr.String())
	}
	var report struct {
		Status       string `json:"status"`
		EarliestLoss struct {
			Code     string `json:"code"`
			Position struct {
				FrameExact string `json:"frameExact"`
			} `json:"position"`
		} `json:"earliestLoss"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &report); err != nil {
		t.Fatal(err)
	}
	if report.Status != "lossy" {
		t.Fatalf("status = %q, want lossy", report.Status)
	}
	if report.EarliestLoss.Code != "TRANSITION_FLATTENED" || report.EarliestLoss.Position.FrameExact != "96" {
		t.Fatalf("earliest loss = %#v", report.EarliestLoss)
	}
	if !strings.Contains(stderr.String(), "earliest loss frame 96") {
		t.Fatalf("summary missing earliest frame: %s", stderr.String())
	}
}

func TestRepairWritesVerifiedFallbackAndStillReturnsNonZero(t *testing.T) {
	output := filepath.Join(t.TempDir(), "repaired.fcpxml")
	var stdout, stderr bytes.Buffer
	code := run([]string{
		"repair",
		"-source", filepath.Join("..", "..", "testdata", "source.fcpxml"),
		"-candidate", filepath.Join("..", "..", "testdata", "lossy.fcpxml"),
		"-out", output,
		"-report", "-",
		"-quiet",
	}, &stdout, &stderr)
	if code != 2 {
		t.Fatalf("exit = %d, want 2; stderr=%s", code, stderr.String())
	}
	data, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	if !strings.Contains(text, "<transition") || !strings.Contains(text, "<timeMap") {
		t.Fatalf("repair output dropped effects: %s", text)
	}
	var report struct {
		Repair struct {
			Strategy       string `json:"strategy"`
			Reimportable   bool   `json:"reimportable"`
			Preserves      bool   `json:"preservesEffects"`
			DeletedEffects int    `json:"deletedEffectCount"`
		} `json:"repair"`
		Summary struct {
			ConversionSucceeded bool `json:"conversionSucceeded"`
		} `json:"summary"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &report); err != nil {
		t.Fatal(err)
	}
	if report.Repair.Strategy != "source_fallback" || !report.Repair.Reimportable || !report.Repair.Preserves {
		t.Fatalf("repair report = %#v", report.Repair)
	}
	if report.Repair.DeletedEffects != 3 {
		t.Fatalf("deleted effects = %d, want 3", report.Repair.DeletedEffects)
	}
	if report.Summary.ConversionSucceeded {
		t.Fatal("lossy conversion was reported as successful")
	}
}

func TestCompatibleAuditReturnsZero(t *testing.T) {
	var stdout, stderr bytes.Buffer
	path := filepath.Join("..", "..", "testdata", "source.fcpxml")
	code := run([]string{"audit", "-source", path, "-candidate", path}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit = %d, want 0; stderr=%s report=%s", code, stderr.String(), stdout.String())
	}
}

func TestUnresolvedSourceReportsNonExpressibleList(t *testing.T) {
	root := filepath.Join("..", "..", "testdata", "source.fcpxml")
	raw, err := os.ReadFile(root)
	if err != nil {
		t.Fatal(err)
	}
	brokenPath := filepath.Join(t.TempDir(), "broken.fcpxml")
	if err := os.WriteFile(brokenPath, []byte(strings.Replace(string(raw), `ref="r2"`, `ref="r99"`, 1)), 0o644); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	code := run([]string{
		"repair", "-source", brokenPath, "-candidate", brokenPath,
		"-out", filepath.Join(t.TempDir(), "must-not-exist.fcpxml"), "-report", "-", "-quiet",
	}, &stdout, &stderr)
	if code != 2 {
		t.Fatalf("exit = %d, want 2", code)
	}
	var report struct {
		Repair struct {
			Strategy       string `json:"strategy"`
			NonExpressible []any  `json:"nonExpressible"`
		} `json:"repair"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &report); err != nil {
		t.Fatal(err)
	}
	if report.Repair.Strategy != "blocked" || len(report.Repair.NonExpressible) == 0 {
		t.Fatalf("blocked repair = %#v", report.Repair)
	}
}
