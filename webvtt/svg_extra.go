package webvtt

import (
	"fmt"
	"math"
	"strings"
	"time"
)

type svgIssueRow struct {
	issue       IssueReport
	idLines     []string
	detailLines []string
	height      float64
}

func buildIssueRows(issues []IssueReport, tableWidth float64) []svgIssueRow {
	const detailOffset = 302.0
	detailWidth := tableWidth - detailOffset - 10
	if detailWidth < 160 {
		detailWidth = 160
	}
	rows := make([]svgIssueRow, 0, len(issues))
	for _, issue := range issues {
		idText := strings.Join(issue.CueIDs, ", ")
		if idText == "" {
			idText = "-"
		}
		detail := issue.Message
		if issue.DurationMS > 0 {
			detail = fmt.Sprintf("%s  (+%dms)  %s", timestampMS(issue.AtMS), issue.DurationMS, detail)
		} else {
			detail = fmt.Sprintf("%s  %s", timestampMS(issue.AtMS), detail)
		}
		idLines := wrapCueLines([]string{idText}, 138, 10)
		detailLines := wrapCueLines([]string{detail}, detailWidth, 10)
		lines := len(idLines)
		if len(detailLines) > lines {
			lines = len(detailLines)
		}
		if lines < 1 {
			lines = 1
		}
		rows = append(rows, svgIssueRow{
			issue:       issue,
			idLines:     idLines,
			detailLines: detailLines,
			height:      16 + float64(lines)*13,
		})
	}
	return rows
}

func writeTimeline(b *strings.Builder, scene svgScene) {
	report := scene.report
	x := scene.screenX
	y := scene.timelineY
	w := scene.width - 2*scene.screenX
	fmt.Fprintf(b, `<rect x="%s" y="%s" width="%s" height="%s" rx="6" fill="#0f172a" stroke="#334155"/>`+"\n",
		ftoa(x), ftoa(y), ftoa(w), ftoa(scene.timelineH))
	fmt.Fprintf(b, `<text x="%s" y="%s" fill="#f8fafc" font-family="DejaVu Sans, sans-serif" font-size="12" font-weight="700">SORTED TIMELINE</text>`+"\n",
		ftoa(x+12), ftoa(y+18))

	minMS, maxMS := cueRange(report.Cues)
	span := maxMS - minMS
	if span <= 0 {
		span = 1
	}
	trackX := x + 132
	trackW := w - 132 - 18
	fmt.Fprintf(b, `<text x="%s" y="%s" text-anchor="end" fill="#64748b" font-family="DejaVu Sans Mono, monospace" font-size="9">%s to %s</text>`+"\n",
		ftoa(x+w-12), ftoa(y+18), timestampMS(minMS), timestampMS(maxMS))
	if len(report.Cues) == 0 {
		fmt.Fprintf(b, `<text x="%s" y="%s" fill="#94a3b8" font-family="DejaVu Sans Mono, monospace" font-size="10">no cues</text>`+"\n",
			ftoa(x+12), ftoa(y+44))
		return
	}
	for index, cue := range report.Cues {
		laneY := y + 36 + float64(index)*21
		barX := trackX + float64(cue.StartMS-minMS)/float64(span)*trackW
		barW := math.Max(2, float64(cue.EndMS-cue.StartMS)/float64(span)*trackW)
		if barX+barW > trackX+trackW {
			barW = trackX + trackW - barX
		}
		fmt.Fprintf(b, `<g data-cue-id="%s" data-start-ms="%d" data-end-ms="%d">`+"\n",
			escapeAttr(cue.ID), cue.StartMS, cue.EndMS)
		fmt.Fprintf(b, `<title>%s: %s to %s</title>`+"\n", escapeText(cue.ID), timestampMS(cue.StartMS), timestampMS(cue.EndMS))
		fmt.Fprintf(b, `<text x="%s" y="%s" fill="#cbd5e1" font-family="DejaVu Sans Mono, monospace" font-size="9">%d %s</text>`+"\n",
			ftoa(x+12), ftoa(laneY+5), index+1, escapeText(truncate(cue.ID, 13)))
		fmt.Fprintf(b, `<line x1="%s" y1="%s" x2="%s" y2="%s" stroke="#1e293b"/>`+"\n",
			ftoa(trackX), ftoa(laneY), ftoa(trackX+trackW), ftoa(laneY))
		fmt.Fprintf(b, `<rect x="%s" y="%s" width="%s" height="9" rx="2" fill="%s" fill-opacity="0.78" stroke="%s" stroke-width="1"/>`+"\n",
			ftoa(barX), ftoa(laneY-5), ftoa(barW), paletteColor(index), ruleColor(cue.Rules))
		b.WriteString(`</g>` + "\n")
	}
	sampleX := trackX + float64(report.SampleTimeMS-minMS)/float64(span)*trackW
	if report.SampleTimeMS < minMS || report.SampleTimeMS > maxMS {
		sampleX = trackX
	}
	fmt.Fprintf(b, `<line x1="%s" y1="%s" x2="%s" y2="%s" stroke="#f87171" stroke-width="1.5"/>`+"\n",
		ftoa(sampleX), ftoa(y+28), ftoa(sampleX), ftoa(y+scene.timelineH-16))
	fmt.Fprintf(b, `<text x="%s" y="%s" fill="#f87171" font-family="DejaVu Sans Mono, monospace" font-size="8">sample</text>`+"\n",
		ftoa(sampleX+3), ftoa(y+scene.timelineH-8))
}

func writeDiagnostics(b *strings.Builder, scene svgScene) {
	x, y := scene.screenX, scene.diagnosticY
	w := scene.width - 2*scene.screenX
	fmt.Fprintf(b, `<text x="%s" y="%s" fill="#f8fafc" font-family="DejaVu Sans, sans-serif" font-size="14" font-weight="700">RULE DIAGNOSTICS</text>`+"\n",
		ftoa(x), ftoa(y+14))
	fmt.Fprintf(b, `<text x="%s" y="%s" text-anchor="end" fill="#94a3b8" font-family="DejaVu Sans Mono, monospace" font-size="10">every row names the triggering cue ID(s)</text>`+"\n",
		ftoa(x+w), ftoa(y+14))

	tableY := y + 28
	fmt.Fprintf(b, `<rect x="%s" y="%s" width="%s" height="26" rx="3" fill="#111827"/>`+"\n",
		ftoa(x), ftoa(tableY), ftoa(w))
	fmt.Fprintf(b, `<text x="%s" y="%s" fill="#94a3b8" font-family="DejaVu Sans Mono, monospace" font-size="9" font-weight="700">RULE</text>`+"\n",
		ftoa(x+12), ftoa(tableY+16))
	fmt.Fprintf(b, `<text x="%s" y="%s" fill="#94a3b8" font-family="DejaVu Sans Mono, monospace" font-size="9" font-weight="700">CUE ID(S)</text>`+"\n",
		ftoa(x+152), ftoa(tableY+16))
	fmt.Fprintf(b, `<text x="%s" y="%s" fill="#94a3b8" font-family="DejaVu Sans Mono, monospace" font-size="9" font-weight="700">TRIGGER / DETAIL</text>`+"\n",
		ftoa(x+302), ftoa(tableY+16))

	rowY := tableY + 26
	if len(scene.issueRows) == 0 {
		fmt.Fprintf(b, `<rect x="%s" y="%s" width="%s" height="38" fill="#0f172a"/>`+"\n",
			ftoa(x), ftoa(rowY), ftoa(w))
		fmt.Fprintf(b, `<text x="%s" y="%s" fill="#4ade80" font-family="DejaVu Sans Mono, monospace" font-size="10">No rule violations.</text>`+"\n",
			ftoa(x+12), ftoa(rowY+23))
		return
	}
	for index, row := range scene.issueRows {
		issue := row.issue
		fill := "#0f172a"
		if index%2 == 1 {
			fill = "#0b1220"
		}
		fmt.Fprintf(b, `<g data-issue-code="%s" data-cue-ids="%s" data-severity="%s">`+"\n",
			escapeAttr(string(issue.Code)), escapeAttr(strings.Join(issue.CueIDs, " ")), escapeAttr(string(issue.Severity)))
		fmt.Fprintf(b, `<rect x="%s" y="%s" width="%s" height="%s" fill="%s"/>`+"\n",
			ftoa(x), ftoa(rowY), ftoa(w), ftoa(row.height), fill)
		fmt.Fprintf(b, `<rect x="%s" y="%s" width="3" height="%s" fill="%s"/>`+"\n",
			ftoa(x), ftoa(rowY), ftoa(row.height), severityColor(issue.Severity))
		fmt.Fprintf(b, `<text x="%s" y="%s" fill="%s" font-family="DejaVu Sans Mono, monospace" font-size="9" font-weight="700">%s</text>`+"\n",
			ftoa(x+12), ftoa(rowY+14), severityColor(issue.Severity), escapeText(string(issue.Code)))
		for lineIndex, line := range row.idLines {
			fmt.Fprintf(b, `<text x="%s" y="%s" fill="#e2e8f0" font-family="DejaVu Sans Mono, monospace" font-size="10" font-weight="600">%s</text>`+"\n",
				ftoa(x+152), ftoa(rowY+14+float64(lineIndex)*13), escapeText(line))
		}
		for lineIndex, line := range row.detailLines {
			fmt.Fprintf(b, `<text x="%s" y="%s" fill="#cbd5e1" font-family="DejaVu Sans Mono, monospace" font-size="10">%s</text>`+"\n",
				ftoa(x+302), ftoa(rowY+14+float64(lineIndex)*13), escapeText(line))
		}
		b.WriteString(`</g>` + "\n")
		rowY += row.height
	}
}

func cueRange(cues []CueReport) (int64, int64) {
	if len(cues) == 0 {
		return 0, 0
	}
	start, end := cues[0].StartMS, cues[0].EndMS
	for _, cue := range cues[1:] {
		if cue.StartMS < start {
			start = cue.StartMS
		}
		if cue.EndMS > end {
			end = cue.EndMS
		}
	}
	return start, end
}

func severityColor(severity Severity) string {
	switch severity {
	case SeverityError:
		return "#f87171"
	case SeverityWarning:
		return "#fbbf24"
	default:
		return "#60a5fa"
	}
}

func durationFromMS(ms int64) time.Duration {
	return time.Duration(ms) * time.Millisecond
}
