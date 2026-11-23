package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func runCLI(t *testing.T, input string) (int, report, string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	code := run([]string{"-in", "-", "-out", "-"}, strings.NewReader(input), &stdout, &stderr)
	var decoded report
	if stdout.Len() != 0 {
		if err := json.Unmarshal(stdout.Bytes(), &decoded); err != nil {
			t.Fatalf("decode report: %v\n%s", err, stdout.String())
		}
	}
	return code, decoded, stderr.String()
}

func TestRunSuccessAndRejectedUpdate(t *testing.T) {
	success := `[
		{"type":"config","sequence":1,"layouts":[],"programs":[],"objects":[]},
		{"type":"update","sequence":2,"layouts":[]}
	]`
	code, decoded, stderr := runCLI(t, success)
	if code != 0 || len(decoded.Updates) != 2 {
		t.Fatalf("code=%d report=%+v stderr=%s", code, decoded, stderr)
	}
	if decoded.Updates[1].EffectiveSequence != 2 || !decoded.Updates[1].Applied {
		t.Fatalf("second update report = %+v", decoded.Updates[1])
	}

	rejected := `[
		{"type":"config","sequence":1,"layouts":[],"programs":[],"objects":[]},
		{"type":"update","sequence":1,"layouts":[]}
	]`
	code, decoded, _ = runCLI(t, rejected)
	if code != 1 || len(decoded.Updates) != 2 || decoded.Updates[1].Applied {
		t.Fatalf("code=%d report=%+v", code, decoded)
	}
}

func TestRunUnresolvedExitCodeAndBadInput(t *testing.T) {
	input := `{"type":"config","sequence":1,"layouts":[],"programs":[],"objects":[],
		"extensions":{"x-unknown":true}}`
	code, decoded, _ := runCLI(t, input)
	if code != 3 || len(decoded.Updates) != 1 || len(decoded.Updates[0].Unresolved) == 0 {
		t.Fatalf("code=%d report=%+v", code, decoded)
	}
	code, _, _ = runCLI(t, `{"type":`)
	if code != 2 {
		t.Fatalf("malformed input exit code = %d", code)
	}
}
