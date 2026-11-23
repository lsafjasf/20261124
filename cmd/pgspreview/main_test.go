package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/lsafjasf/20261124/pgs"
)

func TestNormalizeArgs(t *testing.T) {
	got := normalizeArgs([]string{
		"input.sup", "0.5", "out.ppm",
		"-width", "1280", "-background=checker", "-json", "r.json",
	})
	want := []string{
		"-width", "1280", "-background=checker", "-json", "r.json",
		"input.sup", "0.5", "out.ppm",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("normalizeArgs = %#v, want %#v", got, want)
	}
}

func TestParseTimestampKeepsExactTicks(t *testing.T) {
	q, err := parseTimestamp("135001t")
	if err != nil {
		t.Fatal(err)
	}
	if !q.usePTS || q.pts != 135001 {
		t.Fatalf("tick query = %+v", q)
	}
	q, err = parseTimestamp("1.5")
	if err != nil {
		t.Fatal(err)
	}
	if q.usePTS || q.duration != 1500*time.Millisecond {
		t.Fatalf("seconds query = %+v", q)
	}
}

func TestParseBackground(t *testing.T) {
	bg, err := parseBackground("#102030", 16)
	if err != nil {
		t.Fatal(err)
	}
	c := bg(0, 0)
	if c.R != 0x10 || c.G != 0x20 || c.B != 0x30 || c.A != 255 {
		t.Fatalf("background = %+v", c)
	}
	if _, err := parseBackground("nope", 16); err == nil {
		t.Fatal("expected invalid background error")
	}
}

func TestRunWritesPPMAndWindowJSON(t *testing.T) {
	out := filepath.Join(t.TempDir(), "frame.ppm")
	var stdout, stderr bytes.Buffer
	err := run([]string{
		"-input", "../../testdata/scenario.sup",
		"-at", "0.5",
		"-width", "1280",
		"-background", "checker",
		"-out", out,
	}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("run: %v (stderr %s)", err, stderr.String())
	}

	var rep report
	if err := json.Unmarshal(stdout.Bytes(), &rep); err != nil {
		t.Fatalf("JSON report: %v\n%s", err, stdout.String())
	}
	if rep.Output.Width != 1280 || rep.Output.Height != 720 {
		t.Fatalf("output = %+v", rep.Output)
	}
	if len(rep.Windows) != 2 || rep.Windows[0].Output != (pgs.Rect{X: 400, Y: 100, Width: 200, Height: 200}) {
		t.Fatalf("windows = %+v", rep.Windows)
	}
	if len(rep.Objects) != 3 {
		t.Fatalf("objects = %d, want 3", len(rep.Objects))
	}

	ppm, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(ppm, []byte("P6\n1280 720\n255\n")) {
		t.Fatalf("bad PPM header %q", ppm[:min(len(ppm), 32)])
	}
}
