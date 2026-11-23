package hdr

import (
	"bytes"
	"encoding/json"
	"math"
	"strings"
	"testing"
)

func TestCompressFrameReportsSeparateRegions(t *testing.T) {
	f := NewFrame(8, 6)
	for i := range f.Pixels {
		f.Pixels[i] = RGB{R: 0.1, G: 0.2, B: 0.3}
	}
	for y := 1; y <= 2; y++ {
		for x := 1; x <= 2; x++ {
			f.Set(x, y, RGB{R: math.NaN(), G: 0, B: 0})
		}
	}
	for y := 4; y <= 5; y++ {
		for x := 5; x <= 6; x++ {
			f.Set(x, y, RGB{R: -0.5, G: -0.1, B: -0.2})
		}
	}

	out, report := CompressFrame(f, Balanced)
	if report.NonFinite != 4 || report.NegativeLuminance != 4 || report.Unrecoverable != 8 {
		t.Fatalf("unexpected counts: %+v", report)
	}
	if len(report.Regions) != 2 {
		t.Fatalf("regions = %d, want 2: %+v", len(report.Regions), report.Regions)
	}
	first, second := report.Regions[0], report.Regions[1]
	if first.Reason != ReasonNonFinite || first.X != 1 || first.Y != 1 ||
		first.Width != 2 || first.Height != 2 || first.Pixels != 4 {
		t.Fatalf("first region = %+v", first)
	}
	if second.Reason != ReasonNegativeLuminance || second.X != 5 || second.Y != 4 ||
		second.Width != 2 || second.Height != 2 || second.Pixels != 4 {
		t.Fatalf("second region = %+v", second)
	}
	for _, c := range out.Pixels {
		assertDisplayable(t, c)
	}
}

func TestCompressFrameCountsNegativeAndHighlight(t *testing.T) {
	f := NewFrame(2, 1)
	f.Set(0, 0, RGB{R: -0.1, G: 0, B: 0.5})
	f.Set(1, 0, RGB{R: 0.1, G: 0.2, B: 2})

	_, report := CompressFrame(f, Balanced)
	if report.NegativeRecovered != 1 {
		t.Fatalf("negative_recovered = %d, want 1", report.NegativeRecovered)
	}
	if report.HighlightCompressed != 1 {
		t.Fatalf("highlight_compressed = %d, want 1", report.HighlightCompressed)
	}
	if report.Unrecoverable != 0 || len(report.Regions) != 0 {
		t.Fatalf("unexpected loss: %+v", report)
	}
}

func TestFindRegionsDoesNotJoinAcrossRowBoundary(t *testing.T) {
	reasons := []Reason{"", "", ReasonNonFinite, ReasonNonFinite, "", ""}
	regions := findRegions(reasons, 3, 2)
	if len(regions) != 2 {
		t.Fatalf("regions = %d, want 2: %+v", len(regions), regions)
	}
	if regions[0].X != 2 || regions[0].Y != 0 || regions[1].X != 0 || regions[1].Y != 1 {
		t.Fatalf("unexpected region positions: %+v", regions)
	}
}

func TestReportLogWritesJSONLinesPerFrame(t *testing.T) {
	var buf bytes.Buffer
	log := NewReportLog(&buf)
	for i := 0; i < 2; i++ {
		idx, err := log.Write(FrameReport{Width: 4, Height: 3, Preset: "balanced"})
		if err != nil {
			t.Fatal(err)
		}
		if idx != i {
			t.Fatalf("frame index = %d, want %d", idx, i)
		}
	}

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) != 2 {
		t.Fatalf("lines = %d, want 2", len(lines))
	}
	for i, line := range lines {
		var record struct {
			Frame int `json:"frame"`
			FrameReport
		}
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			t.Fatalf("line %d: %v", i, err)
		}
		if record.Frame != i || record.Preset != "balanced" || record.Width != 4 {
			t.Fatalf("line %d decoded = %+v", i, record)
		}
	}
}
