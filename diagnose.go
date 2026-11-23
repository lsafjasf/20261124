package main

import (
	"fmt"
	"math"
	"strings"
)

// FitResult is the outcome of fitting one transfer hypothesis to a set of
// (input, output) intermediate pixel pairs.
type FitResult struct {
	Hypothesis string
	Gain       float64 // fitted linear gain (1 for pure curves)
	RMS        float64 // root-mean-square residual
	MaxErr     float64
}

type hypothesis struct {
	name string
	// apply maps input x with linear gain a through the candidate transfer.
	apply func(x, a float64) float64
}

var hypotheses = []hypothesis{
	{"identity", func(x, a float64) float64 { return x }},
	{"gain-only", func(x, a float64) float64 { return a * x }},
	{"tonemap", func(x, a float64) float64 { return ToneMap(a * x) }},
	{"tonemap^2", func(x, a float64) float64 { return ToneMap(ToneMap(a * x)) }},
	{"srgb-encode", func(x, a float64) float64 { return SRGBEncode(a * x) }},
}

// residuals evaluates a hypothesis with gain a over paired samples.
func residuals(h hypothesis, xs, ys []float64, a float64) (rms, maxErr float64) {
	var sum float64
	for i := range xs {
		d := h.apply(xs[i], a) - ys[i]
		sum += d * d
		if math.Abs(d) > maxErr {
			maxErr = math.Abs(d)
		}
	}
	return math.Sqrt(sum / float64(len(xs))), maxErr
}

// fitGain finds the best linear gain for a hypothesis by golden-section
// search on log10(a) in [-3, 3].
func fitGain(h hypothesis, xs, ys []float64) (bestA, bestRMS float64) {
	lo, hi := -3.0, 3.0
	const gr = 0.6180339887498949
	c, d := hi-gr*(hi-lo), lo+gr*(hi-lo)
	fc, _ := residuals(h, xs, ys, math.Pow(10, c))
	fd, _ := residuals(h, xs, ys, math.Pow(10, d))
	for i := 0; i < 200; i++ {
		if fc > fd {
			lo = c
			c, fc = d, fd
			d = lo + gr*(hi-lo)
			fd, _ = residuals(h, xs, ys, math.Pow(10, d))
		} else {
			hi = d
			d, fd = c, fc
			c = hi - gr*(hi-lo)
			fc, _ = residuals(h, xs, ys, math.Pow(10, c))
		}
	}
	bestA = math.Pow(10, (lo+hi)/2)
	bestRMS, _ = residuals(h, xs, ys, bestA)
	return bestA, bestRMS
}

// FitStage fits every hypothesis to one stage boundary and returns results
// sorted best-first.
func FitStage(in, out *Frame) []FitResult {
	xs := in.Pix
	ys := out.Pix
	results := make([]FitResult, 0, len(hypotheses))
	for _, h := range hypotheses {
		a, rms := fitGain(h, xs, ys)
		_, mx := residuals(h, xs, ys, a)
		results = append(results, FitResult{Hypothesis: h.name, Gain: a, RMS: rms, MaxErr: mx})
	}
	for i := 1; i < len(results); i++ {
		for j := i; j > 0 && results[j].RMS < results[j-1].RMS; j-- {
			results[j], results[j-1] = results[j-1], results[j]
		}
	}
	return results
}

// DiagnoseResult summarizes one frame.
type FrameDiagnosis struct {
	Frame     int
	Shot      string
	StageBest map[string]FitResult // stage boundary -> best hypothesis
	DoubleAt  []string             // boundaries best explained by tonemap^2
	ExtraTMAt []string             // boundaries that apply an unexpected tone map
}

// DiagnoseFrame walks the stage boundaries of one dump and classifies each.
func DiagnoseFrame(d *StageDump) FrameDiagnosis {
	fd := FrameDiagnosis{
		Frame:     d.Params.Frame,
		Shot:      d.Params.Shot,
		StageBest: map[string]FitResult{},
	}
	for i := 0; i+1 < len(d.Order); i++ {
		a, b := d.Order[i], d.Order[i+1]
		best := FitStage(d.Stages[a], d.Stages[b])[0]
		name := a + "->" + b
		fd.StageBest[name] = best
		switch {
		case best.Hypothesis == "tonemap^2":
			fd.DoubleAt = append(fd.DoubleAt, name)
		case best.Hypothesis == "tonemap" && name != "exposure->tonemap":
			// a tone map anywhere but the dedicated stage is unexpected
			fd.ExtraTMAt = append(fd.ExtraTMAt, name)
		}
	}
	return fd
}

// FormatStageTable renders per-boundary fit tables for one dump.
func FormatStageTable(d *StageDump) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "frame %d (%s, gain=%.4g, HighlightProtect=%v)\n",
		d.Params.Frame, d.Params.Shot, d.Params.ExposureGain, d.Params.HighlightProtect)
	for i := 0; i+1 < len(d.Order); i++ {
		a, b := d.Order[i], d.Order[i+1]
		fmt.Fprintf(&sb, "  %s -> %s\n", a, b)
		for _, r := range FitStage(d.Stages[a], d.Stages[b]) {
			fmt.Fprintf(&sb, "    %-12s gain=%.6g  rms=%.3e  max=%.3e\n",
				r.Hypothesis, r.Gain, r.RMS, r.MaxErr)
		}
	}
	return sb.String()
}
