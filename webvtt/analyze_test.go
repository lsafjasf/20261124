package webvtt

import (
	"math"
	"strings"
	"testing"
)

func mustParse(t *testing.T, input string) *Document {
	t.Helper()
	doc, err := Parse(strings.NewReader(input))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	return doc
}

func mustAnalyze(t *testing.T, input string, mutate func(*Options)) *Report {
	t.Helper()
	opts := DefaultOptions()
	if mutate != nil {
		mutate(&opts)
	}
	report, err := Analyze(mustParse(t, input), opts)
	if err != nil {
		t.Fatalf("Analyze() error = %v", err)
	}
	return report
}

func findIssue(report *Report, code Rule, cueIDs ...string) *IssueReport {
	for index := range report.Issues {
		issue := &report.Issues[index]
		if issue.Code != code || len(issue.CueIDs) != len(cueIDs) {
			continue
		}
		matches := true
		for i := range cueIDs {
			if issue.CueIDs[i] != cueIDs[i] {
				matches = false
				break
			}
		}
		if matches {
			return issue
		}
	}
	return nil
}

func findCue(report *Report, id string) *CueReport {
	for index := range report.Cues {
		if report.Cues[index].ID == id {
			return &report.Cues[index]
		}
	}
	return nil
}

func TestSameTimestampConflictKeepsSourceOrder(t *testing.T) {
	report := mustAnalyze(t, `WEBVTT

REGION
id:lower
width:60%
lines:2
regionanchor:0%,100%
viewportanchor:20%,90%

C2
00:00:02.000 --> 00:00:04.000 region:lower
second

C1
00:00:02.000 --> 00:00:04.000 region:lower
first
`, nil)

	if got := []string{report.Cues[0].ID, report.Cues[1].ID}; got[0] != "C2" || got[1] != "C1" {
		t.Fatalf("equal timestamps did not keep source order: %v", got)
	}
	if got := report.ActiveCueIDs; len(got) != 2 || got[0] != "C2" || got[1] != "C1" {
		t.Fatalf("active cue IDs = %v, want [C2 C1]", got)
	}
	issue := findIssue(report, RuleConflict, "C2", "C1")
	if issue == nil {
		t.Fatalf("missing CONFLICT issue for C2/C1; issues=%+v", report.Issues)
	}
	if issue.AtMS != 2000 || issue.DurationMS != 2000 {
		t.Fatalf("conflict timing = %d/%dms, want 2000/2000ms", issue.AtMS, issue.DurationMS)
	}
}

func TestAdjacentMillisecondCuesDoNotConflict(t *testing.T) {
	report := mustAnalyze(t, `WEBVTT

A
00:00:01.000 --> 00:00:02.000
first

B
00:00:02.001 --> 00:00:03.000
second
`, nil)

	if len(report.Cues) != 2 || report.Cues[0].ID != "A" || report.Cues[1].ID != "B" {
		t.Fatalf("unexpected sorted cues: %+v", report.Cues)
	}
	for _, issue := range report.Issues {
		if issue.Code == RuleConflict {
			t.Fatalf("adjacent 1ms cues produced conflict: %+v", issue)
		}
	}
	if report.Cues[0].EndMS != 2000 || report.Cues[1].StartMS != 2001 {
		t.Fatalf("boundary changed: end=%d nextStart=%d", report.Cues[0].EndMS, report.Cues[1].StartMS)
	}
}

func TestNegativeMarginIsClampedAndReported(t *testing.T) {
	report := mustAnalyze(t, `WEBVTT

STYLE
.speaker { color: #fbbf24; margin: -12px -4px 0 2px; }

NEG
00:00:01.000 --> 00:00:02.000
<c.speaker>negative margin</c>
`, nil)

	cue := findCue(report, "NEG")
	if cue == nil {
		t.Fatal("cue NEG not found")
	}
	if cue.Margin.Top != 0 || cue.Margin.Left != 2 || cue.Margin.Right != 0 {
		t.Fatalf("clamped margin = %+v, want top/right zero and left 2", cue.Margin)
	}
	issue := findIssue(report, RuleNegativeMargin, "NEG")
	if issue == nil {
		t.Fatalf("missing NEGATIVE_MARGIN issue; issues=%+v", report.Issues)
	}
	if !strings.Contains(issue.Message, "top=-12px") || !strings.Contains(issue.Message, "right=-4px") {
		t.Fatalf("diagnostic does not name negative sides: %q", issue.Message)
	}
	if findIssue(report, RuleMissingStyle, "NEG") != nil {
		t.Fatal("defined .speaker style was reported as missing")
	}
}

func TestMissingStyleFallsBackAndReportsCueID(t *testing.T) {
	report := mustAnalyze(t, `WEBVTT

MISS
00:00:01.000 --> 00:00:02.000
<c.unknown>fallback text</c>
`, nil)

	cue := findCue(report, "MISS")
	if cue == nil {
		t.Fatal("cue MISS not found")
	}
	if cue.Color != "#f5f7ff" {
		t.Fatalf("fallback color = %q, want #f5f7ff", cue.Color)
	}
	issue := findIssue(report, RuleMissingStyle, "MISS")
	if issue == nil {
		t.Fatalf("missing MISSING_STYLE issue; issues=%+v", report.Issues)
	}
	if !strings.Contains(issue.Message, ".unknown") {
		t.Fatalf("missing style diagnostic does not identify selector: %q", issue.Message)
	}
}

func TestUnsortedCuesAreSortedAndReported(t *testing.T) {
	report := mustAnalyze(t, `WEBVTT

LATE
00:00:05.000 --> 00:00:06.000
late

EARLY
00:00:01.000 --> 00:00:02.000
early
`, nil)

	if got := report.Cues[0].ID; got != "EARLY" {
		t.Fatalf("first sorted cue = %q, want EARLY", got)
	}
	issue := findIssue(report, RuleTimeSort, "EARLY", "LATE")
	if issue == nil {
		t.Fatalf("missing TIME_SORT issue; issues=%+v", report.Issues)
	}
}

func TestRegionAnchorsPlaceAndClampBox(t *testing.T) {
	report := mustAnalyze(t, `WEBVTT

REGION
id:centered
width:50%
lines:2
regionanchor:100%,100%
viewportanchor:50%,50%

CENTER
00:00:01.000 --> 00:00:02.000 region:centered
centered
`, nil)
	want := Rect{X: 8, Y: 38, Width: 152, Height: 52}
	assertRect(t, report.Regions[0].Rect, want)
	if report.Regions[0].Clamped {
		t.Fatal("centered region should not be clamped")
	}

	clamped := mustAnalyze(t, `WEBVTT

REGION
id:off
width:50%
lines:2
regionanchor:100%,100%
viewportanchor:0%,0%

EDGE
00:00:01.000 --> 00:00:02.000 region:off
edge
`, nil)
	assertRect(t, clamped.Regions[0].Rect, Rect{X: 8, Y: 8, Width: 152, Height: 52})
	if !clamped.Regions[0].Clamped {
		t.Fatal("off-screen anchor was not clamped")
	}
	if findIssue(clamped, RuleRegionClamp, "EDGE") == nil {
		t.Fatalf("missing REGION_CLAMP issue; issues=%+v", clamped.Issues)
	}
}

func TestLineLimitTruncatesAndReportsCueID(t *testing.T) {
	report := mustAnalyze(t, `WEBVTT

LONG
00:00:01.000 --> 00:00:02.000
This subtitle is deliberately long enough to need several wrapped lines in a very narrow preview.
`, nil)
	cue := findCue(report, "LONG")
	if cue == nil {
		t.Fatal("cue LONG not found")
	}
	if !cue.Truncated || cue.WrappedLineCount <= report.Viewport.MaxLines {
		t.Fatalf("line limit not applied: truncated=%v wrapped=%d limit=%d", cue.Truncated, cue.WrappedLineCount, report.Viewport.MaxLines)
	}
	if cue.LineCount != report.Viewport.MaxLines {
		t.Fatalf("visible lines = %d, want %d", cue.LineCount, report.Viewport.MaxLines)
	}
	if !strings.HasSuffix(cue.Lines[len(cue.Lines)-1], "\u2026") {
		t.Fatalf("last visible line lacks ellipsis: %q", cue.Lines[len(cue.Lines)-1])
	}
	if findIssue(report, RuleLineLimit, "LONG") == nil {
		t.Fatalf("missing LINE_LIMIT issue; issues=%+v", report.Issues)
	}
}

func assertRect(t *testing.T, got, want Rect) {
	t.Helper()
	const epsilon = 0.01
	if math.Abs(got.X-want.X) > epsilon ||
		math.Abs(got.Y-want.Y) > epsilon ||
		math.Abs(got.Width-want.Width) > epsilon ||
		math.Abs(got.Height-want.Height) > epsilon {
		t.Fatalf("rect = %+v, want %+v", got, want)
	}
}
