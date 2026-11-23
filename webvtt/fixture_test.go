package webvtt

import (
	"os"
	"testing"
)

func TestNarrowFixtureCoversAllRules(t *testing.T) {
	file, err := os.Open("../testdata/narrow.vtt")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	doc, err := Parse(file)
	if err != nil {
		t.Fatal(err)
	}
	report, err := Analyze(doc, DefaultOptions())
	if err != nil {
		t.Fatal(err)
	}
	if report.SampleTimeMS != 2000 || len(report.ActiveCueIDs) != 2 {
		t.Fatalf("sample=%d active=%v, want 2000ms and two active cues", report.SampleTimeMS, report.ActiveCueIDs)
	}
	wantRules := []Rule{
		RuleTimeSort,
		RuleLineLimit,
		RuleNegativeMargin,
		RuleMissingStyle,
		RuleRegionClamp,
		RuleConflict,
	}
	if len(report.Issues) != len(wantRules) {
		t.Fatalf("issues=%+v, want one each of %v", report.Issues, wantRules)
	}
	for index, rule := range wantRules {
		if report.Issues[index].Code != rule {
			t.Fatalf("issue[%d] = %s, want %s", index, report.Issues[index].Code, rule)
		}
	}
	if findIssue(report, RuleConflict, "same-time-b", "same-time-a") == nil {
		t.Fatal("fixture conflict does not name both same-time cue IDs")
	}
}
