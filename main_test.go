package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestRunJSONOutput(t *testing.T) {
	var stdout, stderr bytes.Buffer
	status := run([]string{"-fps", "25", "-pretty=false"}, strings.NewReader("{0}{25}x\n"), &stdout, &stderr)
	if status != 0 {
		t.Fatalf("status = %d, stderr = %q", status, stderr.String())
	}
	var result Result
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("invalid JSON output: %v\n%s", err, stdout.String())
	}
	if result.CueCount != 1 || result.Cues[0].EndMS != 1000 {
		t.Fatalf("unexpected result: %+v", result)
	}
}

func TestRunFailOnIssues(t *testing.T) {
	var stdout, stderr bytes.Buffer
	status := run([]string{"-fps", "25", "-fail-on-issues"}, strings.NewReader("{5}{1}x\n"), &stdout, &stderr)
	if status != 1 {
		t.Fatalf("status = %d, want 1", status)
	}
	if !strings.Contains(stdout.String(), `"negative_interval"`) {
		t.Fatalf("missing issue in output: %s", stdout.String())
	}
}

func TestRunRejectsBadArguments(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if status := run([]string{"-pretty=false"}, strings.NewReader(""), &stdout, &stderr); status != 2 {
		t.Fatalf("missing fps status = %d, want 2", status)
	}
	if status := run([]string{"-fps", "25", "a", "b"}, strings.NewReader(""), &stdout, &stderr); status != 2 {
		t.Fatalf("extra argument status = %d, want 2", status)
	}
}
