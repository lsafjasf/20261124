package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func buildBinary(t *testing.T) string {
	t.Helper()
	binary := filepath.Join(t.TempDir(), "editchange")
	cmd := exec.Command("go", "build", "-o", binary, ".")
	cmd.Env = append(os.Environ(), "GOFLAGS=-mod=mod")
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build binary: %v\n%s", err, output)
	}
	return binary
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

const cliProject = `{
  "id": "p",
  "tracks": [
    {"id": "video", "clips": [
      {"id": "c1", "start": 0, "length": 2000, "sourceIn": 0},
      {"id": "c2", "start": 2000, "length": 3000, "sourceIn": 0}
    ]},
    {"id": "subs", "parentId": "video", "clips": [
      {"id": "s1", "start": 300, "length": 1000, "sourceIn": 0}
    ]}
  ]
}`

func TestCLIApplyAndUndoRoundTrip(t *testing.T) {
	binary := buildBinary(t)
	dir := t.TempDir()
	projectPath := filepath.Join(dir, "project.json")
	orderPath := filepath.Join(dir, "order.json")
	journalPath := filepath.Join(dir, "journal.json")
	writeFile(t, projectPath, cliProject)
	writeFile(t, orderPath, `{
	  "id": "o",
	  "operations": [
	    {"clipId": "c2", "action": "trimStart", "delta": 200},
	    {"clipId": "c2", "action": "trimEnd", "delta": -400}
	  ]
	}`)

	apply := exec.Command(binary, "apply", "-project", projectPath, "-order", orderPath,
		"-journal", journalPath, "-mode", "abort")
	if output, err := apply.CombinedOutput(); err != nil {
		t.Fatalf("apply failed: %v\n%s", err, output)
	}
	applied, err := os.ReadFile(projectPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(applied), `"start": 2200`) {
		t.Fatalf("trim was not applied:\n%s", applied)
	}
	if _, err := os.Stat(journalPath); err != nil {
		t.Fatalf("journal not persisted: %v", err)
	}

	undo := exec.Command(binary, "undo", "-project", projectPath, "-journal", journalPath)
	if output, err := undo.CombinedOutput(); err != nil {
		t.Fatalf("undo failed: %v\n%s", err, output)
	}
	restored, err := os.ReadFile(projectPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(restored), `"start": 2000`) ||
		!strings.Contains(string(restored), `"length": 3000`) {
		t.Fatalf("undo did not restore the baseline:\n%s", restored)
	}
}

func TestCLIConflictAbortKeepsProjectIntact(t *testing.T) {
	binary := buildBinary(t)
	dir := t.TempDir()
	projectPath := filepath.Join(dir, "project.json")
	orderPath := filepath.Join(dir, "order.json")
	writeFile(t, projectPath, cliProject)
	writeFile(t, orderPath, `{
	  "id": "o",
	  "operations": [{"clipId": "c1", "action": "trimEnd", "delta": -800}]
	}`)

	cmd := exec.Command(binary, "apply", "-project", projectPath, "-order", orderPath, "-mode", "abort")
	output, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("expected abort, got success:\n%s", output)
	}
	if !strings.Contains(string(output), "aborted") {
		t.Fatalf("unexpected error output:\n%s", output)
	}
	current, err := os.ReadFile(projectPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(current)) != strings.TrimSpace(cliProject) {
		t.Fatalf("project changed after abort:\n%s", current)
	}
}

func TestCLIDemoConverges(t *testing.T) {
	binary := buildBinary(t)
	output, err := exec.Command(binary, "demo").CombinedOutput()
	if err != nil {
		t.Fatalf("demo failed: %v\n%s", err, output)
	}
	if !strings.Contains(string(output), "三种顺序收敛到同一工程状态") {
		t.Fatalf("demo did not prove convergence:\n%s", output)
	}
}
