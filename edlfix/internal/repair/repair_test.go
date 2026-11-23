package repair

import (
	"strings"
	"testing"

	"edlfix/internal/edl"
	"edlfix/internal/rate"
)

const mixedEDL = `TITLE: MIXED DF/NDF SAMPLE
FCM: NON-DROP FRAME

001  REEL_A V     C        00:00:59:28 00:01:00:00 00:00:59:28 00:01:00:00
* COMMENT LINE SHOULD SURVIVE
002  REEL_A V     C        00:01:00:00 00:02:00:00 00:01:00:00 00:02:00:00
003  REEL_X V     C        00:05:00:00 00:05:10:00 00:02:00:00 00:02:10:00
`

func dropRate(t *testing.T) rate.Rate {
	t.Helper()
	r, err := rate.Resolve("29.97")
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestRepairMixedEDL(t *testing.T) {
	doc := edl.Parse(mixedEDL)
	if len(doc.Events) != 3 {
		t.Fatalf("events = %d, want 3", len(doc.Events))
	}
	drop := dropRate(t)
	res := Run(doc, Options{DefaultRate: &drop})

	if res.FirstInconsistent == nil {
		t.Fatal("expected a first inconsistent timecode finding")
	}
	first := res.FirstInconsistent
	// Event 001 line is line 4 (1-based), first token at column 26.
	if first.Line != 4 || first.EventNum != "001" || first.Field != "SRC_IN" {
		t.Fatalf("first finding loc = line %d event %s field %s",
			first.Line, first.EventNum, first.Field)
	}
	if first.Code != "NDF_LABEL_ON_DROP_RATE" || first.Original != "00:00:59:28" {
		t.Fatalf("unexpected first finding %+v", first)
	}
	// NDF 00:00:59:28 at 29.97 is frame 1798, whose canonical DF label is ;28.
	if first.Repaired != "00:00:59;28" || first.FrameCount != 1798 {
		t.Fatalf("repair = %s frames=%d", first.Repaired, first.FrameCount)
	}

	// Event 002 first token NDF 00:01:00:00 = frame 1800 -> canonical 00:01:00;02.
	out := res.RepairedText
	if !strings.Contains(out, "00:01:00;02 00:02:00;04 00:01:00;02 00:02:00;04") {
		t.Fatalf("event 002 not repaired as expected:\n%s", out)
	}
	if !strings.Contains(out, "* COMMENT LINE SHOULD SURVIVE") {
		t.Fatal("non-event content was modified")
	}
	if !strings.Contains(out, "FCM: NON-DROP FRAME") {
		t.Fatal("FCM header must be preserved verbatim")
	}
	if res.RepairedEvents != 3 {
		t.Fatalf("repaired events = %d, want 3", res.RepairedEvents)
	}
}

func TestUnknownReelKeptVerbatim(t *testing.T) {
	doc := edl.Parse(mixedEDL)
	known := dropRate(t)
	res := Run(doc, Options{ReelRates: map[string]rate.Rate{"REEL_A": known}})

	if len(res.Skipped) == 0 {
		t.Fatal("expected skipped entries")
	}
	var skip *SkipReason
	for i := range res.Skipped {
		if res.Skipped[i].Code == "RATE_UNKNOWN" {
			skip = &res.Skipped[i]
		}
	}
	if skip == nil || skip.Reel != "REEL_X" {
		t.Fatalf("REEL_X should be skipped, got %+v", res.Skipped)
	}
	if !strings.Contains(res.RepairedText,
		"003  REEL_X V     C        00:05:00:00 00:05:10:00 00:02:00:00 00:02:10:00") {
		t.Fatalf("skipped event must remain verbatim:\n%s", res.RepairedText)
	}
	if res.SkippedEvents != 1 || res.RepairedEvents != 2 {
		t.Fatalf("counts repaired=%d skipped=%d", res.RepairedEvents, res.SkippedEvents)
	}
}

func TestIllegalDropLabelKeptVerbatim(t *testing.T) {
	// 00:01:00;00 is an illegal 29.97 DF label (that frame number is skipped).
	bad := "001  REEL_A V     C        00:01:00;00 00:01:00;02 00:01:00;00 00:01:00;02\n"
	doc := edl.Parse(bad)
	drop := dropRate(t)
	res := Run(doc, Options{DefaultRate: &drop})
	if len(res.Skipped) != 1 || res.Skipped[0].Code != "ILLEGAL_LABEL" {
		t.Fatalf("expected ILLEGAL_LABEL skip, got %+v", res.Skipped)
	}
	if res.RepairedText != bad {
		t.Fatalf("illegal event must be byte-identical:\n%q\n%q", res.RepairedText, bad)
	}
	if res.RepairedEvents != 0 {
		t.Fatal("no event may be repaired when label is illegal")
	}
}

func TestDropLabelOnNDFRateKeptVerbatim(t *testing.T) {
	bad := "001  REEL_B V     C        00:01:00;02 00:01:01;02 00:01:00;02 00:01:01;02\n"
	doc := edl.Parse(bad)
	ndf, err := rate.Resolve("25")
	if err != nil {
		t.Fatal(err)
	}
	res := Run(doc, Options{DefaultRate: &ndf})
	if len(res.Skipped) != 1 || res.Skipped[0].Code != "DF_ON_NONDROP_RATE" {
		t.Fatalf("expected DF_ON_NONDROP_RATE, got %+v", res.Skipped)
	}
	if res.RepairedText != bad {
		t.Fatal("event must remain verbatim")
	}
}

func TestConsistentEDLUnchanged(t *testing.T) {
	clean := "001  REEL_A V     C        00:00:59;29 00:01:00;02 00:00:59;29 00:01:00;02\n"
	doc := edl.Parse(clean)
	drop := dropRate(t)
	res := Run(doc, Options{DefaultRate: &drop})
	if len(res.Findings) != 0 || res.RepairedText != clean || res.RepairedEvents != 0 {
		t.Fatalf("consistent EDL must be untouched: findings=%+v", res.Findings)
	}
}

func TestEditIntentFrameInvariance(t *testing.T) {
	// Same physical segment expressed first in NDF, then canonical DF.
	ndfLine := "001  REEL_A V     C        00:10:00:00 00:10:01:00 00:10:00:00 00:10:01:00\n"
	doc := edl.Parse(ndfLine)
	drop := dropRate(t)
	res := Run(doc, Options{DefaultRate: &drop})
	// Source and record durations (frame counts) must be preserved exactly.
	for _, f := range res.Findings {
		if f.Field == "SRC_IN" && f.FrameCount != 18000 {
			t.Fatalf("SRC_IN frame count drifted: %d", f.FrameCount)
		}
	}
	if !strings.Contains(res.RepairedText, "00:10:00;18") {
		t.Fatalf("expected canonical DF label 00:10:00;18:\n%s", res.RepairedText)
	}
	// One-second source duration at 29.97 = 30 frames, unchanged after repair.
	doc2 := edl.Parse(res.RepairedText)
	res2 := Run(doc2, Options{DefaultRate: &drop})
	if len(res2.Findings) != 0 {
		t.Fatalf("repaired output must be stable/idempotent: %+v", res2.Findings)
	}
}

func TestFCMSectionConflictWarnsButKeepsLabel(t *testing.T) {
	// Labels are already canonical DF for 29.97, but the section header
	// declares NON-DROP. Labels must remain byte-identical; only a warning
	// is emitted.
	raw := "FCM: NON-DROP FRAME\n" +
		"001  REEL_A V     C        00:00:59;29 00:01:00;02 00:00:59;29 00:01:00;02\n"
	doc := edl.Parse(raw)
	drop := dropRate(t)
	res := Run(doc, Options{DefaultRate: &drop})
	if res.RepairedEvents != 0 {
		t.Fatalf("no rewrite expected, got %d repaired events", res.RepairedEvents)
	}
	if res.RepairedText != raw {
		t.Fatal("canonical labels must stay byte-identical despite FCM conflict")
	}
	warnings := 0
	for _, f := range res.Findings {
		if f.Code == "FCM_SECTION_CONFLICT" && f.Severity == SeverityWarning {
			warnings++
		}
	}
	if warnings == 0 {
		t.Fatalf("expected FCM_SECTION_CONFLICT warnings, got %+v", res.Findings)
	}
	if res.FirstInconsistent != nil {
		t.Fatal("a warning must not be reported as the first inconsistent timecode")
	}
}

func Test5994DropFrameRepair(t *testing.T) {
	r, err := rate.Resolve("59.94")
	if err != nil {
		t.Fatal(err)
	}
	// NDF 00:01:00:00 at nominal 60 = frame 3600 -> canonical DF 00:01:00;04.
	raw := "001  REEL_H V     C        00:01:00:00 00:01:01:00 00:01:00:00 00:01:01:00\n"
	doc := edl.Parse(raw)
	res := Run(doc, Options{DefaultRate: &r})
	if res.FirstInconsistent == nil || res.FirstInconsistent.Repaired != "00:01:00;04" {
		t.Fatalf("unexpected first finding: %+v", res.FirstInconsistent)
	}
	if !strings.Contains(res.RepairedText, "00:01:00;04") {
		t.Fatalf("59.94 repair wrong: %s", res.RepairedText)
	}
}
