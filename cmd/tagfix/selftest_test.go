package main

import (
	"os"
	"testing"
)

func TestSelfTestCoversFixableAndUnrepairableConflicts(t *testing.T) {
	report := runConflicts(t.TempDir())
	if len(report.Cases) != 4 {
		t.Fatalf("cases = %d, want 4", len(report.Cases))
	}
	if report.AutoFixed != 2 || report.CannotAutoFix != 2 {
		t.Fatalf("fixed=%d cannot=%d", report.AutoFixed, report.CannotAutoFix)
	}
	if !report.AllPayloadsKept {
		t.Fatal("media payload changed in at least one case")
	}
	for _, result := range report.Cases {
		if result.Report == nil {
			t.Errorf("%s has no report", result.Name)
			continue
		}
		if !result.ExpectedFix && !result.OriginalKept {
			t.Errorf("%s did not preserve the original", result.Name)
		}
		if result.ExpectedFix {
			for _, payload := range result.Report.Payload {
				if !payload.Unchanged {
					t.Errorf("%s payload digest changed", result.Name)
				}
			}
		}
	}
}

func TestSelfTestWritesFiles(t *testing.T) {
	dir := t.TempDir()
	report := runConflicts(dir)
	for _, result := range report.Cases {
		if _, err := os.Stat(result.File); err != nil {
			t.Errorf("%s: %v", result.File, err)
		}
		if _, err := os.Stat(result.OriginalFile); err != nil {
			t.Errorf("%s: %v", result.OriginalFile, err)
		}
	}
}
