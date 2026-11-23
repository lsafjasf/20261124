package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAnalyzeAndRoundTripCommands(t *testing.T) {
	fixture := filepath.Join("..", "..", "testdata", "nested.fcpxml")
	var stdout, stderr bytes.Buffer
	if code := run([]string{"analyze", fixture}, &stdout, &stderr); code != 0 {
		t.Fatalf("analyze exit = %d, stderr = %s", code, stderr.String())
	}
	output := stdout.String()
	for _, expected := range []string{"top sequence: Top", "frame duration: 1/30s", "kind=asset-clip"} {
		if !strings.Contains(output, expected) {
			t.Errorf("analyze output does not contain %q:\n%s", expected, output)
		}
	}

	stdout.Reset()
	stderr.Reset()
	if code := run([]string{"roundtrip", fixture}, &stdout, &stderr); code != 0 {
		t.Fatalf("roundtrip exit = %d, stderr = %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "preserved") {
		t.Fatalf("unexpected roundtrip output: %s", stdout.String())
	}
}

func TestExportCommandWritesReimportableFile(t *testing.T) {
	fixture := filepath.Join("..", "..", "testdata", "nested.fcpxml")
	outputPath := filepath.Join(t.TempDir(), "exported.fcpxml")
	var stdout, stderr bytes.Buffer
	if code := run([]string{"export", fixture, outputPath}, &stdout, &stderr); code != 0 {
		t.Fatalf("export exit = %d, stderr = %s", code, stderr.String())
	}
	stdout.Reset()
	stderr.Reset()
	if code := run([]string{"analyze", outputPath}, &stdout, &stderr); code != 0 {
		t.Fatalf("analyze export exit = %d, stderr = %s", code, stderr.String())
	}
}

func TestUnknownCommandIsUsageError(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := run([]string{"unknown"}, &stdout, &stderr); code != 2 {
		t.Fatalf("unknown command exit = %d, want 2", code)
	}
}

func TestAnalyzeReportsMissingReference(t *testing.T) {
	fixture := filepath.Join("..", "..", "testdata", "nested.fcpxml")
	data, err := os.ReadFile(fixture)
	if err != nil {
		t.Fatal(err)
	}
	broken := strings.Replace(string(data), `ref="r12"`, `ref="not-there"`, 1)
	path := filepath.Join(t.TempDir(), "broken.fcpxml")
	if err := os.WriteFile(path, []byte(broken), 0o644); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	if code := run([]string{"analyze", path}, &stdout, &stderr); code != 1 {
		t.Fatalf("analyze broken exit = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "missing-reference") {
		t.Fatalf("missing reference not reported: %s", stderr.String())
	}
}
