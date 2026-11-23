package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const cliEDL = "TITLE: CLI\n" +
	"001  CAM_A V     C        00:00:59:28 00:01:00:00 00:00:59:28 00:01:00:00\n" +
	"002  CAM_Z V     C        00:05:00:00 00:05:10:00 00:01:00:00 00:01:10:00\n"

func writeTemp(t *testing.T, name, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestCLIRepairAndSkip(t *testing.T) {
	in := writeTemp(t, "in.edl", cliEDL)
	rates := writeTemp(t, "rates.txt", "CAM_A=29.97\n")
	out := filepath.Join(t.TempDir(), "out.edl")
	var stdout bytes.Buffer

	err := run(config{input: in, ratesFile: rates, output: out}, &stdout, &bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}
	report := stdout.String()
	if !strings.Contains(report, "首个不一致时间码") ||
		!strings.Contains(report, "事件总数: 2 | 已修复: 1 | 原样保留: 1") {
		t.Fatalf("report missing key content:\n%s", report)
	}
	if !strings.Contains(report, "RATE_UNKNOWN") {
		t.Fatal("skipped CAM_Z reason must appear in report")
	}
	fixed, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	text := string(fixed)
	if !strings.Contains(text, "00:00:59;28 00:01:00;02") {
		t.Fatalf("repaired labels missing:\n%s", text)
	}
	// Skipped event stays byte-identical.
	if !strings.Contains(text,
		"002  CAM_Z V     C        00:05:00:00 00:05:10:00 00:01:00:00 00:01:10:00") {
		t.Fatal("skipped event was modified")
	}
}

func TestCLIDryRunWritesNothing(t *testing.T) {
	in := writeTemp(t, "in.edl", cliEDL)
	out := filepath.Join(t.TempDir(), "should-not-exist.edl")
	var stdout bytes.Buffer
	err := run(config{input: in, defaultFPS: "29.97", output: out, dryRun: true},
		&stdout, &bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(out); !os.IsNotExist(err) {
		t.Fatal("dry-run must not create the output file")
	}
}

func TestCLIMissingRateErrors(t *testing.T) {
	in := writeTemp(t, "in.edl", cliEDL)
	err := run(config{input: in}, &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "帧率") {
		t.Fatalf("expected rate configuration error, got %v", err)
	}
}

func TestCLIJSONOutput(t *testing.T) {
	in := writeTemp(t, "in.edl", cliEDL)
	var stdout bytes.Buffer
	err := run(config{input: in, defaultFPS: "29.97", reportJSON: true, dryRun: true},
		&stdout, &bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}
	json := stdout.String()
	if !strings.Contains(json, "\"firstInconsistent\"") ||
		!strings.Contains(json, "\"NDF_LABEL_ON_DROP_RATE\"") ||
		!strings.Contains(json, "\"totalEvents\": 2") {
		t.Fatalf("JSON output malformed:\n%s", json)
	}
}
