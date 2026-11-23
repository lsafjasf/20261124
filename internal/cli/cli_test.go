package cli

import (
	"bytes"
	"strings"
	"testing"
)

func TestRunUsageAndReadFailures(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := Run("stlcheck", nil, &out, &errOut); code != 2 {
		t.Fatalf("no arguments: got exit %d", code)
	}
	if !strings.Contains(errOut.String(), "Usage:") {
		t.Fatalf("usage not written: %q", errOut.String())
	}

	out.Reset()
	errOut.Reset()
	if code := Run("stlcheck", []string{"--", "does-not-exist.stl"}, &out, &errOut); code != 2 {
		t.Fatalf("missing file: got exit %d", code)
	}
	if !strings.Contains(errOut.String(), "no such file") {
		t.Fatalf("read error not written: %q", errOut.String())
	}
}

func TestRunHelp(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := Run("stlcheck", []string{"-h"}, &out, &errOut); code != 0 {
		t.Fatalf("help: got exit %d", code)
	}
	if !strings.Contains(errOut.String(), "Usage:") {
		t.Fatalf("help not written: %q", errOut.String())
	}
}
