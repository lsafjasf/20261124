package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunWritesReproducibleSVGAndJSON(t *testing.T) {
	dir := t.TempDir()
	svgPath := filepath.Join(dir, "preview.svg")
	jsonPath := filepath.Join(dir, "preview.json")
	var stdout, stderr bytes.Buffer
	err := run([]string{
		"-input", filepath.Join("..", "..", "testdata", "narrow.vtt"),
		"-svg", svgPath,
		"-json", jsonPath,
	}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("run() error = %v; stderr=%s", err, stderr.String())
	}
	svg, err := os.ReadFile(svgPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(svg), `data-issue-code="CONFLICT" data-cue-ids="same-time-b same-time-a"`) {
		t.Fatal("SVG conflict row does not name both cue IDs")
	}
	if !strings.Contains(stdout.String(), "CONFLICT") ||
		!strings.Contains(stdout.String(), "same-time-b, same-time-a") {
		t.Fatalf("stdout does not name conflict cue IDs: %s", stdout.String())
	}
	jsonData, err := os.ReadFile(jsonPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(jsonData), `"sampleTimeMs": 2000`) ||
		!strings.Contains(string(jsonData), `"fontSize": 16`) {
		t.Fatalf("unexpected JSON report: %s", jsonData)
	}
}
