package webvtt

import (
	"sort"
	"strings"
	"time"
	"unicode"
)

// SortCues returns a stable timeline order. Equal start/end values keep source order.
func SortCues(cues []Cue) []Cue {
	sorted := append([]Cue(nil), cues...)
	sort.SliceStable(sorted, func(i, j int) bool {
		if sorted[i].Start != sorted[j].Start {
			return sorted[i].Start < sorted[j].Start
		}
		if sorted[i].End != sorted[j].End {
			return sorted[i].End < sorted[j].End
		}
		return sorted[i].SourceIndex < sorted[j].SourceIndex
	})
	return sorted
}

func implicitRegion(maxLines int) Region {
	return Region{
		ID:             "default",
		Width:          100,
		Lines:          maxLines,
		RegionAnchor:   Point{X: 0, Y: 100},
		ViewportAnchor: Point{X: 0, Y: 100},
	}
}

func regionForCue(doc *Document, cue Cue, opts Options) (Region, bool) {
	if cue.Settings.Region == "" {
		return implicitRegion(opts.Viewport.MaxLines), false
	}
	region, ok := doc.Regions[cue.Settings.Region]
	if !ok {
		return implicitRegion(opts.Viewport.MaxLines), true
	}
	return region, false
}

type placedRegion struct {
	Region   Region
	Rect     Rect
	Clamped  bool
	Original Rect
}

func placeRegion(region Region, opts Options) placedRegion {
	v := opts.Viewport
	contentW := v.Width - 2*v.SafeMargin
	contentH := v.Height - 2*v.SafeMargin
	boxW := contentW * region.Width / 100
	boxH := float64(region.Lines)*v.LineHeight + 2*v.Padding

	originalW, originalH := boxW, boxH
	if boxW > contentW {
		boxW = contentW
	}
	if boxH > contentH {
		boxH = contentH
	}
	x := v.SafeMargin + contentW*region.ViewportAnchor.X/100 - boxW*region.RegionAnchor.X/100
	y := v.SafeMargin + contentH*region.ViewportAnchor.Y/100 - boxH*region.RegionAnchor.Y/100
	original := Rect{X: x, Y: y, Width: boxW, Height: boxH}

	clampedX := clamp(x, v.SafeMargin, v.SafeMargin+contentW-boxW)
	clampedY := clamp(y, v.SafeMargin, v.SafeMargin+contentH-boxH)
	rect := Rect{X: clampedX, Y: clampedY, Width: boxW, Height: boxH}
	return placedRegion{
		Region:   region,
		Rect:     rect,
		Clamped:  clampedX != x || clampedY != y || boxW != originalW || boxH != originalH,
		Original: original,
	}
}

func clamp(value, low, high float64) float64 {
	if high < low {
		high = low
	}
	if value < low {
		return low
	}
	if value > high {
		return high
	}
	return value
}

func layoutCue(cue Cue, regionID string, rect Rect, opts Options, style ResolvedStyle) CueLayout {
	v := opts.Viewport
	limit := opts.Viewport.MaxLines
	maxWidth := rect.Width - 2*v.Padding - style.Margin.Left - style.Margin.Right
	if maxWidth < v.FontSize {
		maxWidth = v.FontSize
	}
	wrapped := wrapCueLines(stripMarkup(cue.Text), maxWidth, v.FontSize)
	if len(wrapped) == 0 {
		wrapped = []string{""}
	}
	visible := wrapped
	truncated := len(wrapped) > limit
	if truncated {
		visible = append([]string(nil), wrapped[:limit]...)
		visible[limit-1] = ellipsize(visible[limit-1], maxWidth, v.FontSize)
	}
	return CueLayout{
		Cue:              cue,
		RegionID:         regionID,
		Rect:             rect,
		Lines:            visible,
		LineCount:        len(visible),
		WrappedLineCount: len(wrapped),
		Truncated:        truncated,
		Style:            style,
	}
}

func wrapCueLines(lines []string, maxWidth, fontSize float64) []string {
	var result []string
	for _, line := range lines {
		line = strings.Join(strings.Fields(line), " ")
		if line == "" {
			continue
		}
		current := ""
		currentWidth := 0.0
		lastSpace := -1
		for _, r := range line {
			width := runeWidth(r, fontSize)
			if current != "" && currentWidth+width > maxWidth {
				if lastSpace >= 0 && lastSpace < len(current) {
					before := strings.TrimSpace(current[:lastSpace])
					after := strings.TrimSpace(current[lastSpace+1:])
					if before != "" {
						result = append(result, before)
					}
					current = after
					currentWidth = textWidth(current, fontSize)
					lastSpace = strings.LastIndexByte(current, ' ')
				} else {
					result = append(result, strings.TrimSpace(current))
					current = ""
					currentWidth = 0
					lastSpace = -1
				}
			}
			current += string(r)
			currentWidth += width
			if unicode.IsSpace(r) {
				lastSpace = len(current) - len(string(r))
			}
		}
		if strings.TrimSpace(current) != "" {
			result = append(result, strings.TrimSpace(current))
		}
	}
	return result
}

func textWidth(text string, fontSize float64) float64 {
	total := 0.0
	for _, r := range text {
		total += runeWidth(r, fontSize)
	}
	return total
}

func runeWidth(r rune, fontSize float64) float64 {
	switch {
	case unicode.IsSpace(r):
		return fontSize * 0.34
	case isCJK(r):
		return fontSize
	case r == 'i' || r == 'l' || r == 'I' || r == '!' || r == '|' || r == '.':
		return fontSize * 0.34
	case r == 'm' || r == 'w' || r == 'M' || r == 'W':
		return fontSize * 0.84
	default:
		return fontSize * 0.61
	}
}

func ellipsize(line string, maxWidth, fontSize float64) string {
	ellipsis := "\u2026"
	limit := maxWidth - textWidth(ellipsis, fontSize)
	if limit <= 0 {
		return ellipsis
	}
	var b strings.Builder
	width := 0.0
	for _, r := range line {
		w := runeWidth(r, fontSize)
		if width+w > limit {
			break
		}
		b.WriteRune(r)
		width += w
	}
	return strings.TrimRight(b.String(), " ") + ellipsis
}

func isActive(cue Cue, at time.Duration) bool {
	return at >= cue.Start && at < cue.End
}

func maxActiveTime(layouts []CueLayout, fallback time.Duration) time.Duration {
	best := fallback
	bestCount := -1
	for _, layout := range layouts {
		count := 0
		for _, candidate := range layouts {
			if candidate.Cue.Start <= layout.Cue.Start && layout.Cue.Start < candidate.Cue.End {
				count++
			}
		}
		if count > bestCount || (count == bestCount && layout.Cue.Start < best) {
			best = layout.Cue.Start
			bestCount = count
		}
	}
	return best
}
