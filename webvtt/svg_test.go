package webvtt

import (
	"bytes"
	"strings"
	"testing"
)

func TestSVGShowsCueIDsAndFixedFont(t *testing.T) {
	report := mustAnalyze(t, `WEBVTT

STYLE
.known { color: #fbbf24; margin: -8px; }

VISIBLE
00:00:01.000 --> 00:00:03.000
<c.known>fixed</c> <c.missing>font</c>

OTHER
00:00:02.000 --> 00:00:04.000
overlap
`, nil)
	first, err := RenderSVG(report)
	if err != nil {
		t.Fatalf("RenderSVG() error = %v", err)
	}
	second, err := RenderSVG(report)
	if err != nil {
		t.Fatalf("RenderSVG() second error = %v", err)
	}
	if !bytes.Equal(first, second) {
		t.Fatal("SVG output is not deterministic")
	}
	svg := string(first)
	if strings.Contains(svg, "scale(") {
		t.Fatal("SVG uses a global scale transform")
	}
	if !strings.Contains(svg, `font-size="16"`) {
		t.Fatal("SVG does not retain the fixed 16px cue font")
	}
	for _, want := range []string{
		`data-cue-id="VISIBLE"`,
		`data-cue-id="OTHER"`,
		`data-issue-code="CONFLICT"`,
		`data-issue-code="MISSING_STYLE"`,
		`data-issue-code="NEGATIVE_MARGIN"`,
		`data-cue-ids="VISIBLE"`,
	} {
		if !strings.Contains(svg, want) {
			t.Fatalf("SVG missing %q", want)
		}
	}
}

func TestSVGRegionClampNamesAllAffectedCues(t *testing.T) {
	report := mustAnalyze(t, `WEBVTT

REGION
id:off
width:60%
lines:2
regionanchor:100%,100%
viewportanchor:0%,0%

ONE
00:00:01.000 --> 00:00:02.000 region:off
one

TWO
00:00:03.000 --> 00:00:04.000 region:off
two
`, nil)
	svg, err := RenderSVG(report)
	if err != nil {
		t.Fatalf("RenderSVG() error = %v", err)
	}
	text := string(svg)
	if !strings.Contains(text, `data-issue-code="REGION_CLAMP" data-cue-ids="ONE TWO"`) {
		t.Fatalf("REGION_CLAMP row does not name ONE and TWO: %s", text)
	}
}
