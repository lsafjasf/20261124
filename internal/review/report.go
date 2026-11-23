package review

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

func WriteJSON(w io.Writer, report *Report) error {
	encoder := json.NewEncoder(w)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(report); err != nil {
		return fmt.Errorf("encode report: %w", err)
	}
	return nil
}

func WriteText(w io.Writer, report *Report) error {
	var b strings.Builder
	fmt.Fprintf(&b, "字幕候选冲突报告 %s\n", report.SchemaVersion)
	fmt.Fprintf(&b, "候选冲突: %d  需人工复核: %d\n", report.ConflictCount, report.ManualReviewCount)
	fmt.Fprintf(&b, "阈值: fps=%.3f 时间重合>=%dms 文本>=%.2f 候选>=%.2f 人工<%.2f 区域人工<%.2f\n",
		report.Thresholds.FPS,
		report.Thresholds.MinOverlapMs,
		report.Thresholds.MinTextSimilarity,
		report.Thresholds.CandidateThreshold,
		report.Thresholds.ManualReviewBelow,
		report.Thresholds.ManualRegionBelow,
	)
	if len(report.Conflicts) == 0 {
		b.WriteString("\n未发现满足阈值的候选冲突。\n")
		_, err := io.WriteString(w, b.String())
		return err
	}

	for _, conflict := range report.Conflicts {
		writeConflictText(&b, conflict)
	}
	_, err := io.WriteString(w, b.String())
	return err
}

func writeConflictText(b *strings.Builder, conflict Conflict) {
	label := "候选"
	if conflict.NeedsManualReview {
		label = "候选/需人工"
	}
	fmt.Fprintf(b, "\n[%s] %s  置信度 %.3f\n", conflict.ID, label, conflict.Confidence)
	fmt.Fprintf(b, "  时间: %s - %s  (帧 %d..%d 半开, %d 帧)\n",
		conflict.TimeSegment.StartTimecode,
		conflict.TimeSegment.EndTimecode,
		conflict.FrameEvidence.Overlap.StartFrame,
		conflict.FrameEvidence.Overlap.EndFrameExclusive,
		conflict.FrameEvidence.Overlap.FrameCount,
	)
	fmt.Fprintf(b, "  区域: 交集 %.4f  [x=%.4f y=%.4f w=%.4f h=%.4f]\n",
		conflict.Region.IntersectionRatio,
		conflict.Region.Intersection.X,
		conflict.Region.Intersection.Y,
		conflict.Region.Intersection.Width,
		conflict.Region.Intersection.Height,
	)
	fmt.Fprintf(b, "  轨道: 强制=%s(%s) cue=%s  完整=%s(%s) cue=%s\n",
		conflict.Tracks.Forced.ID,
		conflict.Tracks.Forced.Source,
		conflict.Tracks.Forced.CueID,
		conflict.Tracks.Full.ID,
		conflict.Tracks.Full.Source,
		conflict.Tracks.Full.CueID,
	)
	fmt.Fprintf(b, "  文本: 相似度=%.4f 有效=%.4f 编辑=%.4f 包含=%.4f 双字组=%.4f 最短=%d字\n",
		conflict.Text.Similarity,
		conflict.Text.EffectiveSimilarity,
		conflict.Text.EditSimilarity,
		conflict.Text.ContainmentScore,
		conflict.Text.BigramDice,
		conflict.Text.ShortestTextRunes,
	)
	fmt.Fprintf(b, "    强制: %s\n", oneLine(conflict.Text.ForcedText, 180))
	fmt.Fprintf(b, "    完整: %s\n", oneLine(conflict.Text.FullText, 180))
	if conflict.NeedsManualReview {
		fmt.Fprintf(b, "  复核原因: %s\n", joinReviewReasons(conflict.ReviewReasons))
	}
	fmt.Fprintf(b, "  帧级证据: 强制 cue 帧[%d,%d) 完整 cue 帧[%d,%d)\n",
		conflict.FrameEvidence.ForcedCue.StartFrame,
		conflict.FrameEvidence.ForcedCue.EndFrameExclusive,
		conflict.FrameEvidence.FullCue.StartFrame,
		conflict.FrameEvidence.FullCue.EndFrameExclusive,
	)
	for _, check := range conflict.FrameEvidence.FrameChecks {
		fmt.Fprintf(b, "    帧 %d: 强制=%t 完整=%t\n", check.Frame, check.ForcedActive, check.FullActive)
	}
	b.WriteString("  修复建议:\n")
	for _, suggestion := range conflict.RepairSuggestions {
		fmt.Fprintf(b, "    %d. %s 轨道=%s cues=%s 帧[%d,%d) 自动=%t\n",
			suggestion.Priority,
			suggestion.Action,
			suggestion.TrackID,
			strings.Join(suggestion.CueIDs, ","),
			suggestion.StartFrame,
			suggestion.EndFrameExclusive,
			suggestion.Automatic,
		)
		fmt.Fprintf(b, "       %s\n", suggestion.Rationale)
	}
}

func oneLine(text string, limit int) string {
	text = strings.Join(strings.Fields(text), " ")
	runes := []rune(text)
	if len(runes) <= limit {
		return text
	}
	return string(runes[:limit]) + "…"
}

func joinReviewReasons(reasons []string) string {
	labels := make([]string, 0, len(reasons))
	for _, reason := range reasons {
		label := reviewReasonLabels[reason]
		if label == "" {
			label = reason
		}
		labels = append(labels, label)
	}
	return strings.Join(labels, "；")
}

var reviewReasonLabels = map[string]string{
	"confidence_below_manual_threshold":      "综合置信度低于人工阈值",
	"text_similarity_below_manual_threshold": "文本相似度低于人工阈值",
	"region_overlap_below_manual_threshold":  "区域交集低于人工阈值",
	"short_text_ambiguous":                   "短文本歧义，需人工判断",
	"partial_timing_overlap":                 "仅部分时间重合",
}
