package review

import (
	"math"
	"slices"
	"testing"
)

func TestAnalyzeProducesFrameEvidenceAndRepairSuggestions(t *testing.T) {
	cfg := testConfig(
		Cue{ID: "f1", StartMs: 1000, EndMs: 3000, Text: "快走，危险！"},
		Cue{ID: "u1", StartMs: 1200, EndMs: 3200, Text: "<i>快走危险</i>"},
	)
	report, err := Analyze(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if report.ConflictCount != 1 {
		t.Fatalf("got %d conflicts, want 1: %+v", report.ConflictCount, report.Conflicts)
	}
	conflict := report.Conflicts[0]
	if conflict.NeedsManualReview {
		t.Fatalf("exact high-confidence candidate should not require manual review: %+v", conflict.ReviewReasons)
	}
	if conflict.FrameEvidence.Overlap.StartFrame != 30 ||
		conflict.FrameEvidence.Overlap.EndFrameExclusive != 75 ||
		conflict.FrameEvidence.Overlap.FrameCount != 45 {
		t.Fatalf("unexpected frame overlap: %+v", conflict.FrameEvidence.Overlap)
	}
	if len(conflict.FrameEvidence.FrameChecks) != 3 {
		t.Fatalf("got %d frame checks, want 3", len(conflict.FrameEvidence.FrameChecks))
	}
	for _, check := range conflict.FrameEvidence.FrameChecks {
		if !check.ForcedActive || !check.FullActive {
			t.Fatalf("frame check is not active on both tracks: %+v", check)
		}
	}
	if len(conflict.RepairSuggestions) != 2 {
		t.Fatalf("got %d suggestions, want 2", len(conflict.RepairSuggestions))
	}
	if conflict.RepairSuggestions[0].Action != "suppress_forced_narrative_overlap" {
		t.Fatalf("unexpected primary repair: %+v", conflict.RepairSuggestions[0])
	}
}

func TestAnalyzeDetectsFuzzyDuplicateWithoutTextEquality(t *testing.T) {
	cfg := testConfig(
		Cue{ID: "f1", StartMs: 1000, EndMs: 3000, Text: "列车即将到站"},
		Cue{ID: "u1", StartMs: 1000, EndMs: 3200, Text: "列车即将到达本站"},
	)
	report, err := Analyze(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if report.ConflictCount != 1 {
		t.Fatalf("got %d conflicts, want 1", report.ConflictCount)
	}
	text := report.Conflicts[0].Text
	if text.NormalizedForced == text.NormalizedFull {
		t.Fatal("test input must not normalize to equal text")
	}
	if text.Similarity <= 0.6 || text.Similarity >= 1 {
		t.Fatalf("expected fuzzy similarity between 0.6 and 1, got %+v", text)
	}
}

func TestAnalyzeSkipsUnrelatedText(t *testing.T) {
	cfg := testConfig(
		Cue{ID: "f1", StartMs: 1000, EndMs: 3000, Text: "请立即疏散"},
		Cue{ID: "u1", StartMs: 1000, EndMs: 3000, Text: "今日天气晴朗"},
	)
	report, err := Analyze(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if report.ConflictCount != 0 {
		t.Fatalf("unrelated text produced conflicts: %+v", report.Conflicts)
	}
}

func TestAnalyzeMarksDisjointRegionForManualReview(t *testing.T) {
	cfg := testConfig(
		Cue{ID: "f1", StartMs: 1000, EndMs: 2000, Text: "注意安全"},
		Cue{ID: "u1", StartMs: 1000, EndMs: 2000, Text: "注意安全"},
	)
	cfg.Tracks[0].Region = &Region{X: 0, Y: 0, Width: 1, Height: 0.1}
	cfg.Tracks[1].Region = &Region{X: 0, Y: 0.9, Width: 1, Height: 0.1}
	report, err := Analyze(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if report.ConflictCount != 1 {
		t.Fatalf("got %d conflicts, want 1", report.ConflictCount)
	}
	conflict := report.Conflicts[0]
	if !conflict.NeedsManualReview {
		t.Fatal("disjoint region should force manual review")
	}
	if !slices.Contains(conflict.ReviewReasons, "region_overlap_below_manual_threshold") {
		t.Fatalf("missing region review reason: %+v", conflict.ReviewReasons)
	}
	if conflict.RepairSuggestions[0].Action != "manual_review" {
		t.Fatalf("first repair should be manual review: %+v", conflict.RepairSuggestions)
	}
}

func TestAnalyzeMarksShortTextForManualReview(t *testing.T) {
	cfg := testConfig(
		Cue{ID: "f1", StartMs: 1000, EndMs: 2000, Text: "是"},
		Cue{ID: "u1", StartMs: 1000, EndMs: 2000, Text: "是"},
	)
	report, err := Analyze(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if report.ConflictCount != 1 {
		t.Fatalf("got %d conflicts, want 1", report.ConflictCount)
	}
	conflict := report.Conflicts[0]
	if !conflict.NeedsManualReview || !conflict.Text.ShortTextAdjusted {
		t.Fatalf("short text was not sent to manual review: %+v", conflict)
	}
}

func TestAnalyzeRequiresTemporalOverlap(t *testing.T) {
	cfg := testConfig(
		Cue{ID: "f1", StartMs: 1000, EndMs: 2000, Text: "同一句话"},
		Cue{ID: "u1", StartMs: 2000, EndMs: 3000, Text: "同一句话"},
	)
	report, err := Analyze(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if report.ConflictCount != 0 {
		t.Fatalf("touching half-open cues must not conflict: %+v", report.Conflicts)
	}
}

func TestCueFrameSpanHalfOpenAtFractionalFPS(t *testing.T) {
	span := cueFrameSpan(Cue{StartMs: 1000, EndMs: 1040}, 29.97)
	if span.StartFrame != 29 || span.EndFrameExclusive != 32 || span.FrameCount != 3 {
		t.Fatalf("unexpected fractional FPS span: %+v", span)
	}
	if math.Abs(float64(span.StartFrame)/29.97*1000-967.63) > 2 {
		t.Fatalf("unexpected frame start mapping: %+v", span)
	}
}

func testConfig(forced, full Cue) *Config {
	region := &Region{X: 0.1, Y: 0.75, Width: 0.8, Height: 0.15}
	return &Config{
		FPS:                  25,
		MinOverlapMs:         120,
		CandidateThreshold:   0.38,
		MinTextSimilarity:    0.34,
		ManualReviewBelow:    0.72,
		ManualRegionBelow:    0.25,
		ManualTextBelow:      0.72,
		ManualShortTextRunes: 3,
		Tracks: []Track{
			{ID: "forced", Kind: "forced_narrative", Source: "forced.srt", Region: region, Cues: []Cue{forced}},
			{ID: "full", Kind: "full", Source: "full.srt", Region: region, Cues: []Cue{full}},
		},
	}
}
