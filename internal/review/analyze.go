package review

import (
	"fmt"
	"math"
	"sort"
)

const reportSchemaVersion = "subtitle-conflict-report/v1"

func Analyze(cfg *Config) (*Report, error) {
	if err := cfg.validate(); err != nil {
		return nil, err
	}

	report := &Report{
		SchemaVersion: reportSchemaVersion,
		Thresholds:    thresholdsOf(cfg),
		Conflicts:     make([]Conflict, 0),
	}

	for fi := range cfg.Tracks {
		forced := &cfg.Tracks[fi]
		if forced.Kind != "forced_narrative" {
			continue
		}
		for ui := range cfg.Tracks {
			full := &cfg.Tracks[ui]
			if full.Kind != "full" {
				continue
			}
			report.Conflicts = append(report.Conflicts, compareTracks(cfg, forced, full)...)
		}
	}

	sort.SliceStable(report.Conflicts, func(i, j int) bool {
		a, b := report.Conflicts[i], report.Conflicts[j]
		if a.TimeSegment.StartMs != b.TimeSegment.StartMs {
			return a.TimeSegment.StartMs < b.TimeSegment.StartMs
		}
		if a.Tracks.Forced.ID != b.Tracks.Forced.ID {
			return a.Tracks.Forced.ID < b.Tracks.Forced.ID
		}
		if a.Tracks.Full.ID != b.Tracks.Full.ID {
			return a.Tracks.Full.ID < b.Tracks.Full.ID
		}
		return a.FrameEvidence.Overlap.StartFrame < b.FrameEvidence.Overlap.StartFrame
	})

	for i := range report.Conflicts {
		report.Conflicts[i].ID = fmt.Sprintf("conflict-%03d", i+1)
		if report.Conflicts[i].NeedsManualReview {
			report.ManualReviewCount++
		}
	}
	report.ConflictCount = len(report.Conflicts)
	return report, nil
}

func thresholdsOf(cfg *Config) Thresholds {
	return Thresholds{
		FPS:                  cfg.FPS,
		MinOverlapMs:         cfg.MinOverlapMs,
		CandidateThreshold:   cfg.CandidateThreshold,
		MinTextSimilarity:    cfg.MinTextSimilarity,
		ManualReviewBelow:    cfg.ManualReviewBelow,
		ManualRegionBelow:    cfg.ManualRegionBelow,
		ManualTextBelow:      cfg.ManualTextBelow,
		ManualShortTextRunes: cfg.ManualShortTextRunes,
		MinRegionOverlap:     cfg.MinRegionOverlap,
	}
}

func compareTracks(cfg *Config, forced, full *Track) []Conflict {
	forcedCues := sortedCues(forced.Cues)
	fullCues := sortedCues(full.Cues)
	conflicts := make([]Conflict, 0)

	for _, fc := range forcedCues {
		for _, uc := range fullCues {
			if uc.StartMs >= fc.EndMs {
				break
			}
			start := maxInt64(fc.StartMs, uc.StartMs)
			end := minInt64(fc.EndMs, uc.EndMs)
			if end-start < cfg.MinOverlapMs || end <= start {
				continue
			}
			conflict, ok := compareCuePair(cfg, forced, full, fc, uc, start, end)
			if ok {
				conflicts = append(conflicts, conflict)
			}
		}
	}
	return conflicts
}

func compareCuePair(
	cfg *Config,
	forced, full *Track,
	fc, uc Cue,
	overlapStart, overlapEnd int64,
) (Conflict, bool) {
	forcedSpan := cueFrameSpan(fc, cfg.FPS)
	fullSpan := cueFrameSpan(uc, cfg.FPS)
	overlapFrames := FrameSpan{
		StartFrame:        maxInt64(forcedSpan.StartFrame, fullSpan.StartFrame),
		EndFrameExclusive: minInt64(forcedSpan.EndFrameExclusive, fullSpan.EndFrameExclusive),
	}
	overlapFrames.FrameCount = overlapFrames.EndFrameExclusive - overlapFrames.StartFrame
	if overlapFrames.FrameCount <= 0 {
		return Conflict{}, false
	}

	region, ok := regionEvidence(effectiveRegion(fc.Region, forced.Region), effectiveRegion(uc.Region, full.Region))
	if !ok || region.IntersectionRatio < cfg.MinRegionOverlap {
		return Conflict{}, false
	}
	text := scoreText(fc.Text, uc.Text, cfg.ManualShortTextRunes)
	if text.EffectiveSimilarity < cfg.MinTextSimilarity {
		return Conflict{}, false
	}

	timingScore := clamp01(float64(overlapEnd-overlapStart) /
		float64(minInt64(fc.EndMs-fc.StartMs, uc.EndMs-uc.StartMs)))
	confidence := clamp01(
		0.55*text.EffectiveSimilarity +
			0.25*timingScore +
			0.20*region.Score,
	)
	if confidence < cfg.CandidateThreshold {
		return Conflict{}, false
	}

	reasons := reviewReasons(cfg, confidence, timingScore, text, region)
	conflict := Conflict{
		TimeSegment: TimeSegment{
			StartMs:        overlapStart,
			EndMsExclusive: overlapEnd,
			StartTimecode:  formatTimecode(overlapStart),
			EndTimecode:    formatTimecode(overlapEnd),
		},
		Region:            region,
		Tracks:            trackSources(forced, full, fc, uc),
		Text:              text,
		Confidence:        round4(confidence),
		NeedsManualReview: len(reasons) > 0,
		ReviewReasons:     reasons,
		FrameEvidence:     frameEvidence(cfg.FPS, forcedSpan, fullSpan, overlapFrames, fc, uc),
	}
	conflict.RepairSuggestions = repairSuggestions(cfg, conflict)
	return conflict, true
}

func effectiveRegion(cueRegion, trackRegion *Region) *Region {
	if cueRegion != nil {
		return cueRegion
	}
	return trackRegion
}

func sortedCues(cues []Cue) []Cue {
	out := append([]Cue(nil), cues...)
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].StartMs != out[j].StartMs {
			return out[i].StartMs < out[j].StartMs
		}
		if out[i].EndMs != out[j].EndMs {
			return out[i].EndMs < out[j].EndMs
		}
		return out[i].ID < out[j].ID
	})
	return out
}

func regionEvidence(forced, full *Region) (RegionEvidence, bool) {
	if forced == nil || full == nil {
		return RegionEvidence{}, false
	}
	x1 := math.Max(forced.X, full.X)
	y1 := math.Max(forced.Y, full.Y)
	x2 := math.Min(forced.X+forced.Width, full.X+full.Width)
	y2 := math.Min(forced.Y+forced.Height, full.Y+full.Height)
	intersection := Region{X: x1, Y: y1}
	if x2 > x1 && y2 > y1 {
		intersection.Width = x2 - x1
		intersection.Height = y2 - y1
	}
	intersection.X = round4(intersection.X)
	intersection.Y = round4(intersection.Y)
	intersection.Width = round4(intersection.Width)
	intersection.Height = round4(intersection.Height)
	smallest := math.Min(forced.Width*forced.Height, full.Width*full.Height)
	ratio := 0.0
	if smallest > 0 {
		ratio = clamp01(intersection.Width * intersection.Height / smallest)
	}
	return RegionEvidence{
		ForcedRegion:      *forced,
		FullRegion:        *full,
		Intersection:      intersection,
		IntersectionRatio: round4(ratio),
		Score:             round4(ratio),
	}, true
}

func frameEvidence(fps float64, forced, full, overlap FrameSpan, fc, uc Cue) FrameEvidence {
	checks := []FrameCheck{
		frameCheck(overlap.StartFrame, forced, full),
		frameCheck(overlap.EndFrameExclusive-1, forced, full),
	}
	if overlap.FrameCount > 2 {
		mid := overlap.StartFrame + overlap.FrameCount/2
		if mid != checks[0].Frame && mid != checks[1].Frame {
			checks = append(checks, frameCheck(mid, forced, full))
		}
	}
	sort.SliceStable(checks, func(i, j int) bool { return checks[i].Frame < checks[j].Frame })
	return FrameEvidence{
		FPS:     fps,
		Overlap: overlap,
		ForcedCue: FrameCue{
			CueID:             fc.ID,
			StartFrame:        forced.StartFrame,
			EndFrameExclusive: forced.EndFrameExclusive,
		},
		FullCue: FrameCue{
			CueID:             uc.ID,
			StartFrame:        full.StartFrame,
			EndFrameExclusive: full.EndFrameExclusive,
		},
		FrameChecks: checks,
	}
}

func frameCheck(frame int64, forced, full FrameSpan) FrameCheck {
	return FrameCheck{
		Frame:        frame,
		ForcedActive: frame >= forced.StartFrame && frame < forced.EndFrameExclusive,
		FullActive:   frame >= full.StartFrame && frame < full.EndFrameExclusive,
	}
}

func cueFrameSpan(cue Cue, fps float64) FrameSpan {
	start := int64(math.Floor(float64(cue.StartMs)*fps/1000 + 1e-9))
	end := int64(math.Ceil(float64(cue.EndMs)*fps/1000 - 1e-9))
	return FrameSpan{StartFrame: start, EndFrameExclusive: end, FrameCount: end - start}
}

func reviewReasons(cfg *Config, confidence, timing float64, text TextEvidence, region RegionEvidence) []string {
	reasons := make([]string, 0, 4)
	if confidence < cfg.ManualReviewBelow {
		reasons = append(reasons, "confidence_below_manual_threshold")
	}
	if text.EffectiveSimilarity < cfg.ManualTextBelow {
		reasons = append(reasons, "text_similarity_below_manual_threshold")
	}
	if region.Score < cfg.ManualRegionBelow {
		reasons = append(reasons, "region_overlap_below_manual_threshold")
	}
	if text.ShortTextAdjusted {
		reasons = append(reasons, "short_text_ambiguous")
	}
	if timing < 0.5 {
		reasons = append(reasons, "partial_timing_overlap")
	}
	return reasons
}

func trackSources(forced, full *Track, fc, uc Cue) TrackSources {
	return TrackSources{
		Forced: TrackRef{
			ID:       forced.ID,
			Kind:     forced.Kind,
			Source:   forced.Source,
			Language: forced.Language,
			CueID:    fc.ID,
		},
		Full: TrackRef{
			ID:       full.ID,
			Kind:     full.Kind,
			Source:   full.Source,
			Language: full.Language,
			CueID:    uc.ID,
		},
	}
}

func repairSuggestions(cfg *Config, conflict Conflict) []RepairSuggestion {
	frames := conflict.FrameEvidence.Overlap
	suggestions := make([]RepairSuggestion, 0, 3)
	if conflict.NeedsManualReview {
		suggestions = append(suggestions, RepairSuggestion{
			Priority:          1,
			Action:            "manual_review",
			TrackID:           conflict.Tracks.Forced.ID,
			CueIDs:            []string{conflict.Tracks.Forced.CueID, conflict.Tracks.Full.CueID},
			StartFrame:        frames.StartFrame,
			EndFrameExclusive: frames.EndFrameExclusive,
			StartTimecode:     conflict.TimeSegment.StartTimecode,
			EndTimecode:       conflict.TimeSegment.EndTimecode,
			Automatic:         false,
			Rationale:         "低置信度候选，先人工逐帧核对字幕内容与排版，不自动修改。",
		})
	}
	priority := len(suggestions) + 1
	suggestions = append(suggestions, RepairSuggestion{
		Priority:          priority,
		Action:            "suppress_forced_narrative_overlap",
		TrackID:           conflict.Tracks.Forced.ID,
		CueIDs:            []string{conflict.Tracks.Forced.CueID},
		StartFrame:        frames.StartFrame,
		EndFrameExclusive: frames.EndFrameExclusive,
		StartTimecode:     conflict.TimeSegment.StartTimecode,
		EndTimecode:       conflict.TimeSegment.EndTimecode,
		Automatic:         false,
		Rationale:         "保留完整字幕，仅在证据帧段内隐藏强制叙述；若整条均重合，可删除整条强制叙述 cue。",
	})
	suggestions = append(suggestions, RepairSuggestion{
		Priority:          priority + 1,
		Action:            "suppress_full_subtitle_overlap",
		TrackID:           conflict.Tracks.Full.ID,
		CueIDs:            []string{conflict.Tracks.Full.CueID},
		StartFrame:        frames.StartFrame,
		EndFrameExclusive: frames.EndFrameExclusive,
		StartTimecode:     conflict.TimeSegment.StartTimecode,
		EndTimecode:       conflict.TimeSegment.EndTimecode,
		Automatic:         false,
		Rationale:         "备选方案：若编辑策略要求保留强制叙述，则在完整字幕轨隐藏同一帧段。",
	})
	return suggestions
}

func formatTimecode(ms int64) string {
	if ms < 0 {
		ms = 0
	}
	hour := ms / 3_600_000
	ms %= 3_600_000
	minute := ms / 60_000
	ms %= 60_000
	second := ms / 1000
	millisecond := ms % 1000
	return fmt.Sprintf("%02d:%02d:%02d,%03d", hour, minute, second, millisecond)
}

func round4(value float64) float64 {
	return math.Round(value*10000) / 10000
}

func minInt64(a, b int64) int64 {
	if a < b {
		return a
	}
	return b
}

func maxInt64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}
