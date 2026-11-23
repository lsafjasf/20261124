package adm

import (
	"fmt"
	"io"
	"sort"
	"strings"
)

var stageNames = map[string]string{
	StageSource:     "S1 源位置",
	StageNormalized: "S2 坐标归一",
	StagePanned:     "S3 布局声像",
	StageDownmix:    "S4 下混",
}

// WriteReport renders the full diagnostic report as Markdown.
func WriteReport(w io.Writer, res *RenderResult, analysis []*ObjectAnalysis, rows []TimelineRow) {
	fmt.Fprintf(w, "# ADM 渲染链路诊断报告\n\n")
	fmt.Fprintf(w, "- 目标布局：`%s`（%d 声道）\n", res.Layout.Name, len(res.Layout.Speakers))
	fmt.Fprintf(w, "- LoRo 下混：%v\n", res.Downmix)
	fmt.Fprintf(w, "- 对象数：%d，时间线行数：%d\n\n", len(analysis), len(rows))
	if len(res.Warnings) > 0 {
		fmt.Fprintf(w, "## 解析警告\n\n")
		for _, s := range res.Warnings {
			fmt.Fprintf(w, "- %s\n", s)
		}
		fmt.Fprintln(w)
	}
	writeStageTable(w, res, analysis)
	writeTimeline(w, res, rows)
	writeUnrenderable(w, analysis)
	writeEvents(w, res)
}

func writeStageTable(w io.Writer, res *RenderResult, analysis []*ObjectAnalysis) {
	fmt.Fprintf(w, "## 分阶段折叠定位\n\n")
	stages := []string{StageSource, StageNormalized, StagePanned}
	if res.Downmix {
		stages = append(stages, StageDownmix)
	}
	header := "| 对象 | 创作方位上限 |"
	sep := "|---|---|"
	for _, st := range stages {
		header += " " + stageNames[st] + " |"
		sep += "---|"
	}
	header += " 首个折叠阶段 |"
	sep += "---|"
	fmt.Fprintln(w, header)
	fmt.Fprintln(w, sep)
	for _, oa := range analysis {
		row := fmt.Sprintf("| %s | az %.0f° / el %.0f° |", oa.Name, oa.AuthoredMaxAz, oa.AuthoredMaxEl)
		for _, st := range stages {
			status := oa.StageStatus[st]
			if status == "" {
				status = "OK"
			}
			row += " " + status + " |"
		}
		collapse := "未折叠"
		if oa.CollapseStage != "" {
			collapse = stageNames[oa.CollapseStage]
		}
		fmt.Fprintf(w, "%s %s |\n", row, collapse)
	}
	fmt.Fprintln(w)
}

func writeTimeline(w io.Writer, res *RenderResult, rows []TimelineRow) {
	fmt.Fprintf(w, "## 统一时间线（位置 / 各通道能量 dB / 预计映射）\n\n")
	fmt.Fprintf(w, "| 时间段 | 对象 | 位置 (az, el) | 各通道能量 (dB) | 预计映射 | 事件 |\n")
	fmt.Fprintf(w, "|---|---|---|---|---|---|\n")
	for _, r := range rows {
		events := strings.Join(r.Events, "；")
		fmt.Fprintf(w, "| %s | %s | (%.1f°, %.1f°) | %s | %s | %s |\n",
			formatRange(r.Start, r.End), r.Object, r.Az, r.El,
			FormatEnergy(r.Energy, res.Layout), strings.Join(r.Mapping, " "), events)
	}
	fmt.Fprintln(w)
}

func writeUnrenderable(w io.Writer, analysis []*ObjectAnalysis) {
	fmt.Fprintf(w, "## 不可表达对象与人工处理建议\n\n")
	found := false
	for _, oa := range analysis {
		if !oa.Unrenderable && oa.Suggestion == "" {
			continue
		}
		found = true
		kind := "建议复核"
		if oa.Unrenderable {
			kind = "不可表达"
		}
		fmt.Fprintf(w, "- **%s**（%s）：%s\n", oa.Name, kind, oa.Suggestion)
	}
	if !found {
		fmt.Fprintf(w, "- 无\n")
	}
	fmt.Fprintln(w)
}

func writeEvents(w io.Writer, res *RenderResult) {
	fmt.Fprintf(w, "## 折叠事件明细\n\n")
	events := append([]FoldEvent{}, res.Events...)
	for _, pb := range res.Blocks {
		events = append(events, pb.Events...)
	}
	sort.Slice(events, func(i, j int) bool {
		if events[i].Object != events[j].Object {
			return events[i].Object < events[j].Object
		}
		return events[i].Stage < events[j].Stage
	})
	if len(events) == 0 {
		fmt.Fprintf(w, "- 无\n")
		return
	}
	for _, ev := range events {
		fmt.Fprintf(w, "- `%s` %s：%s\n", ev.Object, stageNames[ev.Stage], ev.Detail)
	}
}

func formatRange(start, end float64) string {
	if end < 0 {
		return fmt.Sprintf("%.2fs → 结束", start)
	}
	return fmt.Sprintf("%.2f–%.2fs", start, end)
}
