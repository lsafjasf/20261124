package audit

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMarkdownReportLocksRootCause(t *testing.T) {
	p := loadSample(t)
	r, err := Run(p)
	if err != nil {
		t.Fatal(err)
	}
	md := r.Markdown()
	for _, want := range []string{
		"首个可观察的",
		"c001",
		"-1 ms",
		"片尾误差为 **-500 ms**",
		"e(frame) = legacy_ms - fixed_ms ≈ -0.041667 * frame",
		"e(t)     ≈ -0.000999 * t",
		"整体加 `+500 ms`",
		"TestGlobalOffsetDoesNotFixEarlyCues",
	} {
		if !strings.Contains(md, want) {
			t.Fatalf("markdown missing %q", want)
		}
	}
	if strings.Contains(md, "NaN") {
		t.Fatal("markdown contains NaN")
	}
}

func TestCSVMatchesCurve(t *testing.T) {
	p := loadSample(t)
	r, err := Run(p)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(r.CSV()), "\n")
	if len(lines) != len(r.Curve)+1 {
		t.Fatalf("CSV has %d rows, want %d", len(lines), len(r.Curve)+1)
	}
	if !strings.HasSuffix(lines[len(lines)-1], ",-500,-11.988012") {
		t.Fatalf("unexpected CSV endpoint: %s", lines[len(lines)-1])
	}
}

func TestSVGReport(t *testing.T) {
	p := loadSample(t)
	r, err := Run(p)
	if err != nil {
		t.Fatal(err)
	}
	svg := r.SVG()
	for _, want := range []string{"<svg", "<polyline", "first -1 ms", "end -500 ms", "</svg>"} {
		if !strings.Contains(svg, want) {
			t.Fatalf("SVG missing %q", want)
		}
	}
	if strings.Contains(svg, "NaN") || strings.Contains(svg, "+Inf") {
		t.Fatal("SVG contains a non-finite coordinate")
	}
}

func TestJSONReport(t *testing.T) {
	p := loadSample(t)
	r, err := Run(p)
	if err != nil {
		t.Fatal(err)
	}
	data, err := r.JSON()
	if err != nil {
		t.Fatal(err)
	}
	var got JSONReport
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	if got.FirstError == nil || got.FirstError.CueID != "c001" || got.FirstError.ErrorMS != -1 {
		t.Fatalf("unexpected first error: %+v", got.FirstError)
	}
	if got.MaxAbsErrorMS != 500 || len(got.Curve) != len(r.Curve) {
		t.Fatalf("unexpected JSON summary: max=%d curve=%d", got.MaxAbsErrorMS, len(got.Curve))
	}
}

func TestReportIsDeterministic(t *testing.T) {
	p := loadSample(t)
	a, err := Run(p)
	if err != nil {
		t.Fatal(err)
	}
	b, err := Run(p)
	if err != nil {
		t.Fatal(err)
	}
	if a.Markdown() != b.Markdown() || a.CSV() != b.CSV() || a.SVG() != b.SVG() {
		t.Fatal("report output is not deterministic")
	}
}

func TestCommittedArtifactsAreCurrent(t *testing.T) {
	p := loadSample(t)
	r, err := Run(p)
	if err != nil {
		t.Fatal(err)
	}
	jsonData, err := r.JSON()
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		path string
		want string
	}{
		{"docs/ERROR_CURVE.md", r.Markdown()},
		{"artifacts/error_curve.csv", r.CSV()},
		{"artifacts/error_curve.svg", r.SVG()},
		{"artifacts/audit.json", string(jsonData) + "\n"},
	} {
		got, err := os.ReadFile(filepath.Join("..", tc.path))
		if err != nil {
			t.Fatalf("read %s: %v", tc.path, err)
		}
		if string(got) != tc.want {
			t.Fatalf("%s is stale; run go run ./cmd/subdrift report", tc.path)
		}
	}
}
