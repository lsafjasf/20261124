package webvtt

import (
	"fmt"
	"html"
	"math"
	"strings"
)

type svgScene struct {
	report      *Report
	width       float64
	height      float64
	screenX     float64
	screenY     float64
	sideX       float64
	sideWidth   float64
	timelineY   float64
	timelineH   float64
	diagnosticY float64
	issueRows   []svgIssueRow
}

// RenderSVG returns a self-contained, deterministic preview. Cue text always
// uses Report.Viewport.FontSize; no global scale transform is emitted.
func RenderSVG(report *Report) ([]byte, error) {
	if report == nil {
		return nil, fmt.Errorf("nil preview report")
	}
	scene := buildScene(report)
	var b strings.Builder
	fmt.Fprintf(&b, `<svg xmlns="http://www.w3.org/2000/svg" width="%.0f" height="%.0f" viewBox="0 0 %.0f %.0f" role="img" aria-labelledby="preview-title preview-desc">`+"\n",
		scene.width, scene.height, scene.width, scene.height)
	b.WriteString(`<title id="preview-title">WebVTT narrow-screen subtitle preview</title>` + "\n")
	fmt.Fprintf(&b, `<desc id="preview-desc">%s. Fixed cue font size %.0fpx. Rule diagnostics identify the triggering cue IDs.</desc>`+"\n",
		escapeText(fmt.Sprintf("%d cues, %d issues", len(report.Cues), len(report.Issues))), report.Viewport.FontSize)
	b.WriteString(`<defs>` + "\n")
	b.WriteString(`<clipPath id="screen-clip"><rect x="0" y="0" width="` + ftoa(report.Viewport.Width) + `" height="` + ftoa(report.Viewport.Height) + `" rx="6"/></clipPath>` + "\n")
	for index := range report.Cues {
		if !cueActiveAt(report.Cues[index], report.SampleTimeMS) {
			continue
		}
		rect := report.Cues[index].Rect
		fmt.Fprintf(&b, `<clipPath id="cue-clip-%d"><rect x="%s" y="%s" width="%s" height="%s" rx="4"/></clipPath>`+"\n",
			index, ftoa(rect.X), ftoa(rect.Y), ftoa(rect.Width), ftoa(rect.Height))
	}
	b.WriteString(`</defs>` + "\n")
	b.WriteString(`<rect width="100%" height="100%" fill="#080d17"/>` + "\n")

	writeHeader(&b, scene)
	writeScreen(&b, scene)
	writeSidebar(&b, scene)
	writeTimeline(&b, scene)
	writeDiagnostics(&b, scene)
	b.WriteString(`</svg>` + "\n")
	return []byte(b.String()), nil
}

func buildScene(report *Report) svgScene {
	const (
		margin    = 24.0
		gap       = 24.0
		headerH   = 64.0
		sideWidth = 320.0
	)
	screenW := report.Viewport.Width
	screenH := report.Viewport.Height
	width := math.Max(680, margin*2+screenW+gap+sideWidth)
	sideX := margin + screenW + gap
	timelineY := headerH + screenH + 34
	timelineH := 44 + math.Max(1, float64(len(report.Cues)))*21
	diagnosticY := timelineY + timelineH + 30

	scene := svgScene{
		report:      report,
		width:       width,
		screenX:     margin,
		screenY:     headerH,
		sideX:       sideX,
		sideWidth:   width - sideX - margin,
		timelineY:   timelineY,
		timelineH:   timelineH,
		diagnosticY: diagnosticY,
		issueRows:   buildIssueRows(report.Issues, width-margin*2),
	}
	tableHeight := 34.0
	for _, row := range scene.issueRows {
		tableHeight += row.height
	}
	if len(scene.issueRows) == 0 {
		tableHeight += 44
	}
	scene.height = diagnosticY + 34 + tableHeight + 20
	return scene
}

func writeHeader(b *strings.Builder, scene svgScene) {
	report := scene.report
	fmt.Fprintf(b, `<text id="preview-title-text" x="24" y="28" fill="#f8fafc" font-family="DejaVu Sans, sans-serif" font-size="18" font-weight="700">WebVTT Narrow Preview</text>`+"\n")
	fmt.Fprintf(b, `<text x="24" y="47" fill="#94a3b8" font-family="DejaVu Sans Mono, monospace" font-size="11">%sx%s viewport  |  cue font %.0fpx FIXED  |  max %d lines  |  sample %s</text>`+"\n",
		ftoa(report.Viewport.Width), ftoa(report.Viewport.Height), report.Viewport.FontSize,
		report.Viewport.MaxLines, timestampMS(report.SampleTimeMS))
	fmt.Fprintf(b, `<text x="%.0f" y="28" text-anchor="end" fill="#cbd5e1" font-family="DejaVu Sans Mono, monospace" font-size="12">%d cues / %d issues</text>`+"\n",
		scene.width-24, len(report.Cues), len(report.Issues))
}

func writeScreen(b *strings.Builder, scene svgScene) {
	report := scene.report
	x, y := scene.screenX, scene.screenY
	w, h := report.Viewport.Width, report.Viewport.Height
	fmt.Fprintf(b, `<g id="viewport" transform="translate(%s %s)">`+"\n", ftoa(x), ftoa(y))
	fmt.Fprintf(b, `<rect x="0" y="0" width="%s" height="%s" rx="6" fill="#0b1220" stroke="#334155" stroke-width="1"/>`+"\n", ftoa(w), ftoa(h))
	fmt.Fprintf(b, `<rect x="%s" y="%s" width="%s" height="%s" rx="3" fill="none" stroke="#1e293b" stroke-dasharray="3 3"/>`+"\n",
		ftoa(report.Viewport.SafeMargin), ftoa(report.Viewport.SafeMargin),
		ftoa(w-2*report.Viewport.SafeMargin), ftoa(h-2*report.Viewport.SafeMargin))
	fmt.Fprintf(b, `<g clip-path="url(#screen-clip)">`+"\n")

	for index, cue := range report.Cues {
		if !cueActiveAt(cue, report.SampleTimeMS) {
			continue
		}
		writeCue(b, report, index, cue)
	}
	writeScreenHUD(b, report)
	b.WriteString(`</g>` + "\n")
	b.WriteString(`</g>` + "\n")
}

func writeCue(b *strings.Builder, report *Report, index int, cue CueReport) {
	rect := cue.Rect
	color := paletteColor(index)
	stroke := "#60a5fa"
	if len(cue.Rules) > 0 {
		stroke = "#fbbf24"
	}
	for _, rule := range cue.Rules {
		if rule == RuleConflict || rule == RuleRegionClamp || rule == RuleMissingRegion {
			stroke = "#f87171"
			break
		}
	}
	fmt.Fprintf(b, `<g data-cue-id="%s" data-rules="%s" data-source-index="%d">`+"\n",
		escapeAttr(cue.ID), escapeAttr(joinRules(cue.Rules)), cue.SourceIndex)
	fmt.Fprintf(b, `<title>%s: %s</title>`+"\n", escapeText(cue.ID), escapeText(joinRulesOrNone(cue.Rules)))
	fmt.Fprintf(b, `<rect x="%s" y="%s" width="%s" height="%s" rx="4" fill="%s" fill-opacity="0.20" stroke="%s" stroke-width="1.5"%s/>`+"\n",
		ftoa(rect.X), ftoa(rect.Y), ftoa(rect.Width), ftoa(rect.Height), color, stroke,
		map[bool]string{true: ` stroke-dasharray="5 3"`, false: ""}[len(cue.Rules) > 0])
	fmt.Fprintf(b, `<g clip-path="url(#cue-clip-%d)">`+"\n", index)
	textX := rect.X + report.Viewport.Padding + cue.Margin.Left
	textY := rect.Y + report.Viewport.Padding + cue.Margin.Top + report.Viewport.FontSize*0.82
	for lineIndex, line := range cue.Lines {
		fmt.Fprintf(b, `<text x="%s" y="%s" fill="%s" font-family="DejaVu Sans Mono, monospace" font-size="%s" font-weight="500" xml:space="preserve">%s</text>`+"\n",
			ftoa(textX), ftoa(textY+float64(lineIndex)*report.Viewport.LineHeight),
			escapeAttr(cue.Color), ftoa(report.Viewport.FontSize), escapeText(line))
	}
	b.WriteString(`</g>` + "\n")
	badgeX := rect.Right() - 8
	badgeY := rect.Y + 9
	fmt.Fprintf(b, `<circle cx="%s" cy="%s" r="7" fill="%s" stroke="#0b1220" stroke-width="1"/>`+"\n",
		ftoa(badgeX), ftoa(badgeY), color)
	fmt.Fprintf(b, `<text x="%s" y="%s" text-anchor="middle" fill="#07111f" font-family="DejaVu Sans Mono, monospace" font-size="9" font-weight="700">%d</text>`+"\n",
		ftoa(badgeX), ftoa(badgeY+3), index+1)
	b.WriteString(`</g>` + "\n")
}

func writeScreenHUD(b *strings.Builder, report *Report) {
	var active []CueReport
	for _, cue := range report.Cues {
		if cueActiveAt(cue, report.SampleTimeMS) {
			active = append(active, cue)
		}
	}
	if len(active) == 0 {
		fmt.Fprintf(b, `<rect x="8" y="8" width="180" height="22" rx="4" fill="#111827" fill-opacity="0.92"/>`+"\n")
		fmt.Fprintf(b, `<text x="16" y="23" fill="#94a3b8" font-family="DejaVu Sans Mono, monospace" font-size="10">no active cues at sample time</text>`+"\n")
		return
	}
	visible := active
	if len(visible) > 4 {
		visible = visible[:4]
	}
	height := 12 + float64(len(visible))*13
	fmt.Fprintf(b, `<rect x="6" y="6" width="%s" height="%s" rx="4" fill="#07111f" fill-opacity="0.92" stroke="#334155"/>`+"\n",
		ftoa(report.Viewport.Width-12), ftoa(height))
	for i, cue := range visible {
		rules := joinRulesOrNone(cue.Rules)
		if len(rules) > 38 {
			rules = rules[:35] + "..."
		}
		fmt.Fprintf(b, `<text x="13" y="%s" fill="#e2e8f0" font-family="DejaVu Sans Mono, monospace" font-size="9">%d %s [%s]</text>`+"\n",
			ftoa(18+float64(i)*13), indexOfCue(report, cue), escapeText(cue.ID), escapeText(rules))
	}
	if len(active) > len(visible) {
		fmt.Fprintf(b, `<text x="%s" y="%s" text-anchor="end" fill="#fbbf24" font-family="DejaVu Sans Mono, monospace" font-size="9">+%d more</text>`+"\n",
			ftoa(report.Viewport.Width-12), ftoa(13+float64(len(visible))*13), len(active)-len(visible))
	}
}

func writeSidebar(b *strings.Builder, scene svgScene) {
	report := scene.report
	x, y, w, h := scene.sideX, scene.screenY, scene.sideWidth, report.Viewport.Height
	fmt.Fprintf(b, `<rect x="%s" y="%s" width="%s" height="%s" rx="6" fill="#0f172a" stroke="#334155"/>`+"\n",
		ftoa(x), ftoa(y), ftoa(w), ftoa(h))
	fmt.Fprintf(b, `<text x="%s" y="%s" fill="#f8fafc" font-family="DejaVu Sans, sans-serif" font-size="13" font-weight="700">ACTIVE CUE IDS</text>`+"\n",
		ftoa(x+12), ftoa(y+22))
	fmt.Fprintf(b, `<text x="%s" y="%s" fill="#64748b" font-family="DejaVu Sans Mono, monospace" font-size="9">%s</text>`+"\n",
		ftoa(x+12), ftoa(y+37), timestampMS(report.SampleTimeMS))

	rowY := y + 54
	active := 0
	for index, cue := range report.Cues {
		if !cueActiveAt(cue, report.SampleTimeMS) {
			continue
		}
		if rowY+18 > y+h-28 {
			break
		}
		fmt.Fprintf(b, `<rect x="%s" y="%s" width="8" height="8" rx="2" fill="%s"/>`+"\n",
			ftoa(x+12), ftoa(rowY-8), paletteColor(index))
		fmt.Fprintf(b, `<text x="%s" y="%s" fill="#e2e8f0" font-family="DejaVu Sans Mono, monospace" font-size="10" font-weight="600">%d  %s</text>`+"\n",
			ftoa(x+26), ftoa(rowY), index+1, escapeText(truncate(cue.ID, 18)))
		fmt.Fprintf(b, `<text x="%s" y="%s" text-anchor="end" fill="%s" font-family="DejaVu Sans Mono, monospace" font-size="8">%s</text>`+"\n",
			ftoa(x+w-10), ftoa(rowY), ruleColor(cue.Rules), escapeText(truncate(joinRulesOrNone(cue.Rules), 22)))
		rowY += 20
		active++
	}
	if active == 0 {
		fmt.Fprintf(b, `<text x="%s" y="%s" fill="#94a3b8" font-family="DejaVu Sans Mono, monospace" font-size="10">none</text>`+"\n",
			ftoa(x+12), ftoa(rowY))
	}
	fmt.Fprintf(b, `<line x1="%s" y1="%s" x2="%s" y2="%s" stroke="#1e293b"/>`+"\n",
		ftoa(x+10), ftoa(y+h-26), ftoa(x+w-10), ftoa(y+h-26))
	fmt.Fprintf(b, `<text x="%s" y="%s" fill="#94a3b8" font-family="DejaVu Sans Mono, monospace" font-size="9">font locked at %.0fpx; conflicts are reported, not shrunk away</text>`+"\n",
		ftoa(x+12), ftoa(y+h-10), report.Viewport.FontSize)
}

func indexOfCue(report *Report, target CueReport) int {
	for index, cue := range report.Cues {
		if cue.SourceIndex == target.SourceIndex {
			return index + 1
		}
	}
	return 0
}

func cueActiveAt(cue CueReport, atMS int64) bool {
	return cue.StartMS <= atMS && atMS < cue.EndMS
}

func paletteColor(index int) string {
	colors := []string{"#38bdf8", "#a78bfa", "#34d399", "#fb923c", "#f472b6", "#facc15"}
	return colors[index%len(colors)]
}

func ruleColor(rules []Rule) string {
	if len(rules) == 0 {
		return "#4ade80"
	}
	for _, rule := range rules {
		if rule == RuleConflict || rule == RuleRegionClamp || rule == RuleMissingRegion {
			return "#f87171"
		}
	}
	return "#fbbf24"
}

func joinRules(rules []Rule) string {
	parts := make([]string, 0, len(rules))
	for _, rule := range rules {
		parts = append(parts, string(rule))
	}
	return strings.Join(parts, " ")
}

func joinRulesOrNone(rules []Rule) string {
	if len(rules) == 0 {
		return "ok"
	}
	return joinRules(rules)
}

func timestampMS(ms int64) string {
	return FormatTimestamp(durationFromMS(ms))
}

func truncate(value string, limit int) string {
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	if limit <= 1 {
		return "\u2026"
	}
	return string(runes[:limit-1]) + "\u2026"
}

func ftoa(value float64) string {
	return strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.2f", value), "0"), ".")
}

func escapeText(value string) string {
	return html.EscapeString(value)
}

func escapeAttr(value string) string {
	return strings.ReplaceAll(html.EscapeString(value), "'", "&#39;")
}
