package inspect

import (
	"fmt"
	"sort"

	"vxcheck/internal/vx"
)

// Run executes the full frame-accurate review and returns the ordered report.
func Run(sdr, hdr *vx.File) Report {
	width := sdr.Header.Width
	sdrFeat := BuildFeatures(sdr, width, sdr.Header.Height)
	hdrFeat := BuildFeatures(hdr, width, hdr.Header.Height)
	al := Align4(sdrFeat, hdrFeat, width)

	var findings []Finding
	findings = append(findings, toneFindings(sdr, al)...)
	findings = append(findings, dropFindings(sdr, hdr, al)...)
	findings = append(findings, CheckFrameNumbers(sdr, hdr, al)...)
	findings = append(findings, CheckSubtitles(sdr, hdr)...)
	findings = append(findings, CheckAudio(sdr, hdr)...)
	findings = append(findings, CheckColorMetadata(sdr, hdr)...)

	sort.SliceStable(findings, func(i, j int) bool {
		return anomalyTime(findings[i]) < anomalyTime(findings[j])
	})

	report := Report{SDR: sdr.Header, HDR: hdr.Header, Findings: findings}
	report.Streams = [2]StreamSummary{summarize(sdr), summarize(hdr)}
	report.Tone = toneStats(al)
	for i := range findings {
		f := findings[i]
		if f.Severity == SevMeta && anomalyTime(f) < 0 {
			report.HeaderAnomalies = append(report.HeaderAnomalies, findings[i])
		}
		if f.Severity != SevNormal && anomalyTime(f) >= 0 &&
			report.FirstTimeline == nil {
			report.FirstTimeline = &f
		}
		if f.Severity != SevNormal && report.FirstAnomaly == nil {
			report.FirstAnomaly = &f
		}
	}
	return report
}

func summarize(f *vx.File) StreamSummary {
	s := StreamSummary{
		Title:      f.Header.Title,
		FrameCount: len(f.Frames),
		Subtitles:  f.Subtitles,
	}
	for _, fr := range f.Frames {
		s.DeclaredNums = append(s.DeclaredNums, fr.Num)
		s.PTS = append(s.PTS, fr.PTS)
		if fr.Cut {
			s.CutTicks = append(s.CutTicks, fr.PTS)
		}
	}
	s.AudioPackets = len(f.Audio)
	if len(f.Audio) > 0 {
		s.AudioFirst = f.Audio[0].SampleStart
		last := f.Audio[len(f.Audio)-1]
		s.AudioLastEnd = last.SampleStart + uint64(len(last.Samples))
	}
	return s
}

func toneFindings(sdr *vx.File, al Alignment) []Finding {
	out := make([]Finding, 0, len(al.Pairs))
	for _, p := range al.Pairs {
		out = append(out, Finding{
			Kind: "tone-difference", Severity: SevNormal, Side: "both",
			SDRTick: int64(p.SDRIndex), HDRTick: int64(p.HDRIndex),
			SDRSec:   sdr.Header.TickToSeconds(int64(p.SDRIndex)),
			HDRSec:   sdr.Header.TickToSeconds(int64(p.SDRIndex)),
			SDRFrame: p.SDRIndex, HDRFrame: p.HDRIndex,
			Detail: fmt.Sprintf("grade tone only: edge diff %.4f, SDR-HDR mean luma %d",
				p.EdgeDiff, p.MeanLumaD),
		})
	}
	return out
}

func dropFindings(sdr, hdr *vx.File, al Alignment) []Finding {
	var out []Finding
	for _, idx := range al.SDROnly {
		fr := sdr.Frames[idx]
		out = append(out, Finding{
			Kind: "frame-dropped-in-hdr", Severity: SevDefect, Side: "hdr",
			SDRTick: fr.PTS, SDRSec: sdr.Header.TickToSeconds(fr.PTS),
			SDRFrame: idx,
			Detail:   "SDR frame present on the shared timeline but absent from HDR (dropped frame)",
		})
	}
	for _, idx := range al.HDROnly {
		fr := hdr.Frames[idx]
		out = append(out, Finding{
			Kind: "frame-extra-in-hdr", Severity: SevDefect, Side: "hdr",
			HDRTick: fr.PTS, HDRSec: hdr.Header.TickToSeconds(fr.PTS),
			HDRFrame: idx,
			Detail:   "HDR frame has no structurally matching SDR frame",
		})
	}
	return out
}

func toneStats(al Alignment) ToneStat {
	var sum, n int
	maxEdge := 0.0
	for _, p := range al.Pairs {
		sum += p.MeanLumaD
		n++
		if p.EdgeDiff > maxEdge {
			maxEdge = p.EdgeDiff
		}
	}
	note := "matched frames differ only in tone: edge geometry identical, luma levels shifted by grade"
	if n == 0 {
		note = "no structurally matched frames"
	}
	mean := 0
	if n > 0 {
		mean = sum / n
	}
	return ToneStat{MatchedPairs: n, MeanLumaDeltaMean: mean, MaxEdgeDiff: maxEdge, Note: note}
}

// anomalyTime orders findings by the earliest affected timeline position.
func anomalyTime(f Finding) float64 {
	if f.Kind == "color-metadata-inconsistent" || f.Kind == "color-metadata-identical" {
		return -1
	}
	if f.SDRSec != 0 {
		return f.SDRSec
	}
	return f.HDRSec
}
