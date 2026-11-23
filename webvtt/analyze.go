package webvtt

import (
	"fmt"
	"math"
	"strings"
	"time"
)

// Analyze sorts cues, lays them out, and produces all diagnostics.
func Analyze(doc *Document, opts Options) (*Report, error) {
	if doc == nil {
		return nil, fmt.Errorf("nil WebVTT document")
	}
	if err := opts.Validate(); err != nil {
		return nil, err
	}
	sorted := SortCues(doc.Cues)
	layouts := make([]CueLayout, 0, len(sorted))
	issues := make([]IssueReport, 0)
	moved := movedCues(doc.Cues, sorted)
	if len(moved) > 0 {
		ids := make([]string, 0, len(moved))
		at := time.Duration(math.MaxInt64)
		for _, cue := range sorted {
			if moved[cue.SourceIndex] {
				ids = append(ids, cue.ID)
				if cue.Start < at {
					at = cue.Start
				}
			}
		}
		if at == time.Duration(math.MaxInt64) {
			at = 0
		}
		issues = append(issues, IssueReport{
			Code:     RuleTimeSort,
			Severity: SeverityInfo,
			CueIDs:   ids,
			Message:  "source order differs from timeline order; cues are stably sorted by start and end time",
			AtMS:     durationMS(at),
		})
	}

	placements := make(map[string]placedRegion)
	regionOrder := make([]string, 0)
	regionCues := make(map[string][]string)
	regionLayouts := make(map[string][]int)

	for _, cue := range sorted {
		region, missingRegion := regionForCue(doc, cue, opts)
		key := regionKey(cue, region, missingRegion)
		if _, ok := placements[key]; !ok {
			placements[key] = placeRegion(region, opts)
			regionOrder = append(regionOrder, key)
		}
		regionCues[key] = append(regionCues[key], cue.ID)
		regionLayouts[key] = append(regionLayouts[key], len(layouts))

		style, missingClasses := ResolveStyle(doc.Styles, cue.Classes)
		negative := negativeMargins(style.Margin)
		if negative != "" {
			style.Margin = clampMargin(style.Margin)
		}
		layout := layoutCue(cue, region.ID, placements[key].Rect, opts, style)
		layouts = append(layouts, layout)

		if moved[cue.SourceIndex] {
			addRule(&layouts[len(layouts)-1], RuleTimeSort)
		}
		if missingRegion {
			issue := IssueReport{
				Code:     RuleMissingRegion,
				Severity: SeverityWarning,
				CueIDs:   []string{cue.ID},
				RegionID: cue.Settings.Region,
				Message:  fmt.Sprintf("cue references undefined region %q; using the default bottom-left region", cue.Settings.Region),
				AtMS:     durationMS(cue.Start),
			}
			issues = append(issues, issue)
			addRule(&layouts[len(layouts)-1], RuleMissingRegion)
		}
		if len(missingClasses) > 0 {
			issues = append(issues, IssueReport{
				Code:     RuleMissingStyle,
				Severity: SeverityWarning,
				CueIDs:   []string{cue.ID},
				Message:  fmt.Sprintf("cue class %s has no STYLE rule; default color is used", quotedClasses(missingClasses)),
				AtMS:     durationMS(cue.Start),
			})
			addRule(&layouts[len(layouts)-1], RuleMissingStyle)
		}
		if negative != "" {
			issues = append(issues, IssueReport{
				Code:     RuleNegativeMargin,
				Severity: SeverityWarning,
				CueIDs:   []string{cue.ID},
				Message:  fmt.Sprintf("negative margin %s was clamped to 0px for layout", negative),
				AtMS:     durationMS(cue.Start),
			})
			addRule(&layouts[len(layouts)-1], RuleNegativeMargin)
		}
		if layout.Truncated {
			issues = append(issues, IssueReport{
				Code:     RuleLineLimit,
				Severity: SeverityWarning,
				CueIDs:   []string{cue.ID},
				Message:  fmt.Sprintf("%d wrapped lines exceed the %d-line limit; the last visible line ends with an ellipsis", layout.WrappedLineCount, layout.LineCount),
				AtMS:     durationMS(cue.Start),
			})
			addRule(&layouts[len(layouts)-1], RuleLineLimit)
		}
	}

	for _, key := range regionOrder {
		placement := placements[key]
		region := placement.Region
		if placement.Clamped {
			issues = append(issues, IssueReport{
				Code:     RuleRegionClamp,
				Severity: SeverityError,
				CueIDs:   append([]string(nil), regionCues[key]...),
				RegionID: region.ID,
				Message: fmt.Sprintf(
					"region %q anchors to %.1f%%,%.1f%% and regionanchor %.1f%%,%.1f%%; the resulting box was clamped to keep it inside the viewport",
					region.ID, region.ViewportAnchor.X, region.ViewportAnchor.Y,
					region.RegionAnchor.X, region.RegionAnchor.Y,
				),
				AtMS: durationMS(regionStart(layouts, regionLayouts[key])),
			})
			for _, index := range regionLayouts[key] {
				addRule(&layouts[index], RuleRegionClamp)
			}
		}
	}

	conflicts := conflictIssues(layouts)
	issues = append(issues, conflicts...)

	at := opts.At
	if opts.AutoAt {
		at = maxActiveTime(layouts, 0)
	}
	active := make([]string, 0)
	for _, layout := range layouts {
		if isActive(layout.Cue, at) {
			active = append(active, layout.Cue.ID)
		}
	}

	report := &Report{
		Viewport: ViewportReport{
			Width:      opts.Viewport.Width,
			Height:     opts.Viewport.Height,
			FontSize:   opts.Viewport.FontSize,
			LineHeight: opts.Viewport.LineHeight,
			Padding:    opts.Viewport.Padding,
			SafeMargin: opts.Viewport.SafeMargin,
			MaxLines:   opts.Viewport.MaxLines,
		},
		SampleTimeMS: durationMS(at),
		AutoSample:   opts.AutoAt,
		Cues:         make([]CueReport, 0, len(layouts)),
		Regions:      make([]RegionReport, 0, len(regionOrder)),
		ActiveCueIDs: active,
		Issues:       issues,
	}
	for _, layout := range layouts {
		report.Cues = append(report.Cues, CueReport{
			ID:               layout.Cue.ID,
			GeneratedID:      layout.Cue.GeneratedID,
			SourceIndex:      layout.Cue.SourceIndex,
			StartMS:          durationMS(layout.Cue.Start),
			EndMS:            durationMS(layout.Cue.End),
			RegionID:         layout.RegionID,
			Rect:             layout.Rect,
			Lines:            layout.Lines,
			LineCount:        layout.LineCount,
			WrappedLineCount: layout.WrappedLineCount,
			Truncated:        layout.Truncated,
			Color:            layout.Style.Color,
			Margin:           layout.Style.Margin,
			Rules:            layout.Rules,
		})
	}
	for _, key := range regionOrder {
		placement := placements[key]
		report.Regions = append(report.Regions, RegionReport{
			ID:             placement.Region.ID,
			WidthPercent:   placement.Region.Width,
			Lines:          placement.Region.Lines,
			RegionAnchor:   placement.Region.RegionAnchor,
			ViewportAnchor: placement.Region.ViewportAnchor,
			CueIDs:         append([]string(nil), regionCues[key]...),
			Rect:           placement.Rect,
			OriginalRect:   placement.Original,
			Clamped:        placement.Clamped,
		})
	}
	return report, nil
}

func regionKey(cue Cue, region Region, missing bool) string {
	if cue.Settings.Region == "" || missing {
		return "\x00implicit"
	}
	return "region:" + region.ID
}

func movedCues(original, sorted []Cue) map[int]bool {
	moved := make(map[int]bool)
	for i := range original {
		if i >= len(sorted) || original[i].SourceIndex != sorted[i].SourceIndex {
			moved[original[i].SourceIndex] = true
		}
	}
	if len(moved) == 0 {
		return moved
	}
	for i := range sorted {
		if i >= len(original) || original[i].SourceIndex != sorted[i].SourceIndex {
			moved[sorted[i].SourceIndex] = true
		}
	}
	return moved
}

func regionStart(layouts []CueLayout, indices []int) time.Duration {
	var start time.Duration
	first := true
	for _, index := range indices {
		if first || layouts[index].Cue.Start < start {
			start = layouts[index].Cue.Start
			first = false
		}
	}
	return start
}

func conflictIssues(layouts []CueLayout) []IssueReport {
	var issues []IssueReport
	for i := 0; i < len(layouts); i++ {
		for j := i + 1; j < len(layouts); j++ {
			if layouts[j].Cue.Start >= layouts[i].Cue.End {
				break
			}
			overlapStart := maxDuration(layouts[i].Cue.Start, layouts[j].Cue.Start)
			overlapEnd := minDuration(layouts[i].Cue.End, layouts[j].Cue.End)
			if overlapEnd <= overlapStart || !layouts[i].Rect.Intersects(layouts[j].Rect) {
				continue
			}
			issues = append(issues, IssueReport{
				Code:       RuleConflict,
				Severity:   SeverityError,
				CueIDs:     []string{layouts[i].Cue.ID, layouts[j].Cue.ID},
				Message:    fmt.Sprintf("cue %q overlaps cue %q in the viewport for %dms", layouts[i].Cue.ID, layouts[j].Cue.ID, durationMS(overlapEnd-overlapStart)),
				AtMS:       durationMS(overlapStart),
				DurationMS: durationMS(overlapEnd - overlapStart),
			})
			addRule(&layouts[i], RuleConflict)
			addRule(&layouts[j], RuleConflict)
		}
	}
	return issues
}

func addRule(layout *CueLayout, rule Rule) {
	for _, existing := range layout.Rules {
		if existing == rule {
			return
		}
	}
	layout.Rules = append(layout.Rules, rule)
}

func negativeMargins(margin Margin) string {
	var parts []string
	for _, side := range []struct {
		name  string
		value float64
	}{
		{"top", margin.Top},
		{"right", margin.Right},
		{"bottom", margin.Bottom},
		{"left", margin.Left},
	} {
		if side.value < 0 {
			parts = append(parts, fmt.Sprintf("%s=%.0fpx", side.name, side.value))
		}
	}
	return strings.Join(parts, ", ")
}

func clampMargin(margin Margin) Margin {
	return Margin{
		Top:    math.Max(0, margin.Top),
		Right:  math.Max(0, margin.Right),
		Bottom: math.Max(0, margin.Bottom),
		Left:   math.Max(0, margin.Left),
	}
}

func quotedClasses(classes []string) string {
	quoted := make([]string, 0, len(classes))
	for _, class := range classes {
		quoted = append(quoted, fmt.Sprintf("%q", "."+class))
	}
	return strings.Join(quoted, ", ")
}

func maxDuration(a, b time.Duration) time.Duration {
	if a > b {
		return a
	}
	return b
}

func minDuration(a, b time.Duration) time.Duration {
	if a < b {
		return a
	}
	return b
}
