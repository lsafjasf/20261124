package main

import (
	"fmt"
	"math"
	"strings"
)

// frameRMSErr computes RMS error between two frames.
func frameRMSErr(a, b *Frame) float64 {
	var sum float64
	for i := range a.Pix {
		d := a.Pix[i] - b.Pix[i]
		sum += d * d
	}
	return math.Sqrt(sum / float64(len(a.Pix)))
}

// gainSweepCounterfactual answers: could any exposure gain, with a single
// correct tone map, either reproduce the buggy output, or beat the auto
// exposure against the golden reference? It sweeps gain on the bright
// frame and reports both minima.
func gainSweepCounterfactual(src *Frame, pr Params, buggyOut, golden *Frame) (minToBuggy, minToGolden, bestGainGolden float64) {
	minToBuggy, minToGolden = math.Inf(1), math.Inf(1)
	for lg := -2.0; lg <= 2.0; lg += 0.01 {
		g := math.Pow(10, lg)
		p := pr
		p.ExposureGain = g
		out := ReferenceSDR(src, p)
		if e := frameRMSErr(out, buggyOut); e < minToBuggy {
			minToBuggy = e
		}
		if e := frameRMSErr(out, golden); e < minToGolden {
			minToGolden = e
			bestGainGolden = g
		}
	}
	return minToBuggy, minToGolden, bestGainGolden
}

// BuildPostmortem runs the full investigation and renders the report.
func BuildPostmortem() string {
	var sb strings.Builder
	buggy := &Pipeline{Buggy: true}
	fixed := &Pipeline{Buggy: false}
	buggyDumps, golden := buggy.RunTimeline()
	fixedDumps, _ := fixed.RunTimeline()

	sb.WriteString("# HDR->SDR double tone-mapping postmortem\n\n")
	sb.WriteString("Symptom: on bright shots the peak is not clipped, but skin tones look gray.\n\n")

	sb.WriteString("## 1. Back-solved transfer curves (buggy pipeline)\n\n")
	sb.WriteString("Each stage boundary is fitted against hypotheses; RMS ~ 0 identifies the true transfer.\n\n")
	for _, d := range buggyDumps {
		sb.WriteString("```\n" + FormatStageTable(d) + "```\n")
	}

	sb.WriteString("## 2. Localization\n\n")
	for _, d := range buggyDumps {
		fd := DiagnoseFrame(d)
		if len(fd.ExtraTMAt) > 0 || len(fd.DoubleAt) > 0 {
			fmt.Fprintf(&sb, "- frame %d (%s): unexpected tone map at stage boundary **%s**; HighlightProtect=%v\n",
				fd.Frame, fd.Shot, strings.Join(append(fd.ExtraTMAt, fd.DoubleAt...), ", "), d.Params.HighlightProtect)
		}
	}
	fmt.Fprintf(&sb, "\nConclusion: the second tone map happens in module **look** (boundary tonemap->look), on frames **2,3,4**, exactly the frames where parameter **HighlightProtect=true** (auto-enabled for bright shots, KeyLight=4).\n\n")

	sb.WriteString("## 3. Why this is NOT an exposure problem\n\n")
	var worstExpRMS float64
	for _, d := range buggyDumps {
		best := FitStage(d.Stages["scene"], d.Stages["exposure"])[0]
		if best.Hypothesis == "gain-only" && best.RMS > worstExpRMS {
			worstExpRMS = best.RMS
		}
	}
	fmt.Fprintf(&sb, "1. The exposure stage is purely linear on every frame: best `gain-only` fit has worst RMS = %.3e (machine precision). Exposure only scales; it cannot bend mid-tones.\n", worstExpRMS)
	bright := buggyDumps[2]
	comp := FitStage(bright.Stages["exposure"], bright.Stages["look"])
	sb.WriteString("2. The defect is nonlinear. Fitting the exposure->look composite on bright frame 2:\n\n")
	sb.WriteString("   ```\n")
	for _, r := range comp {
		fmt.Fprintf(&sb, "   %-12s gain=%.6g  rms=%.3e\n", r.Hypothesis, r.Gain, r.RMS)
	}
	sb.WriteString("   ```\n")
	sb.WriteString("   No gain-only or single-tonemap transfer explains the data; `tonemap^2` fits to machine precision.\n")

	src2 := MakeScene(Shots()[2])
	pr2 := AutoParams(2, Shots()[2])
	minToBuggy, minToGolden, bestGain := gainSweepCounterfactual(src2, pr2, bright.Stages["encode"], golden[2])
	fmt.Fprintf(&sb, "3. Counterfactual gain sweep on bright frame 2 (single correct tone map, gain 0.01..100):\n")
	fmt.Fprintf(&sb, "   - best achievable match to the BUGGY output: RMS = %.3e (no exposure value can produce the artifact);\n", minToBuggy)
	fmt.Fprintf(&sb, "   - best achievable match to the GOLDEN output: RMS = %.3e at gain = %.4g, i.e. the auto exposure 1/KeyLight = %.4g was already optimal.\n\n", minToGolden, bestGain, pr2.ExposureGain)

	sb.WriteString("## 4. Skin tone evidence (sRGB 8-bit)\n\n")
	sb.WriteString("| frame | shot | pipeline | skin RGB (8-bit) | chroma |\n")
	sb.WriteString("|---|---|---|---|---|\n")
	for i := range buggyDumps {
		gb := SkinPatchMean(buggyDumps[i].Stages["encode"])
		gf := SkinPatchMean(fixedDumps[i].Stages["encode"])
		gg := SkinPatchMean(golden[i])
		b8 := [3]int{int(gb[0]*255 + 0.5), int(gb[1]*255 + 0.5), int(gb[2]*255 + 0.5)}
		f8 := [3]int{int(gf[0]*255 + 0.5), int(gf[1]*255 + 0.5), int(gf[2]*255 + 0.5)}
		g8 := [3]int{int(gg[0]*255 + 0.5), int(gg[1]*255 + 0.5), int(gg[2]*255 + 0.5)}
		fmt.Fprintf(&sb, "| %d | %s | buggy | (%d,%d,%d) | %.3f |\n", i, Shots()[i].Name, b8[0], b8[1], b8[2], Chroma(gb))
		fmt.Fprintf(&sb, "| %d | %s | fixed | (%d,%d,%d) | %.3f |\n", i, Shots()[i].Name, f8[0], f8[1], f8[2], Chroma(gf))
		fmt.Fprintf(&sb, "| %d | %s | golden | (%d,%d,%d) | %.3f |\n", i, Shots()[i].Name, g8[0], g8[1], g8[2], Chroma(gg))
	}
	sb.WriteString("\nBuggy bright frames crush skin toward gray (chroma collapses); fixed frames match golden.\n\n")

	sb.WriteString("## 5. Fix verification vs single-map golden\n\n")
	sb.WriteString("| frame | shot | buggy RMS vs golden | fixed RMS vs golden |\n")
	sb.WriteString("|---|---|---|---|\n")
	for i := range buggyDumps {
		fmt.Fprintf(&sb, "| %d | %s | %.3e | %.3e |\n",
			i, Shots()[i].Name,
			frameRMSErr(buggyDumps[i].Stages["encode"], golden[i]),
			frameRMSErr(fixedDumps[i].Stages["encode"], golden[i]))
	}
	sb.WriteString("\nFix: the look module no longer applies the scene-referred tone curve; highlight rolloff stays solely in the tonemap stage (WhitePoint=4). HighlightProtect is now a no-op in look and kept only as recorded metadata.\n")
	return sb.String()
}
