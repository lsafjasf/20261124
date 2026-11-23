package compare

import (
	"math"
	"sort"

	"hdrthumb/hdr"
)

// Metrics summarizes one source/output pair. Hue error is circular and is
// measured only on pixels whose source has meaningful chroma.
type Metrics struct {
	Pixels               int     `json:"pixels"`
	ChromaticPixels      int     `json:"chromatic_pixels"`
	HueShiftMeanDeg      float64 `json:"hue_shift_mean_deg"`
	HueShiftP95Deg       float64 `json:"hue_shift_p95_deg"`
	HueShiftMaxDeg       float64 `json:"hue_shift_max_deg"`
	InputNegativePixels  int     `json:"input_negative_pixels"`
	InputOverRangePixels int     `json:"input_over_range_pixels"`
	InputOutOfGamut      int     `json:"input_out_of_gamut_pixels"`
	InputNegativeChans   int     `json:"input_negative_channels"`
	InputOverRangeChans  int     `json:"input_over_range_channels"`
	OutputClippedChans   int     `json:"output_clipped_channels"`
	OutputOutOfGamut     int     `json:"output_out_of_gamut_pixels"`
	MeanLuminance        float64 `json:"mean_luminance"`
	MaxOutput            float64 `json:"max_output"`
}

// Measure compares a source HDR frame with an SDR result.
func Measure(src, out hdr.Frame) Metrics {
	m := Metrics{Pixels: len(src.Pixels)}
	if len(src.Pixels) != len(out.Pixels) {
		return m
	}
	shifts := make([]float64, 0, len(src.Pixels))
	var hueSum, hueWeight float64

	for i, in := range src.Pixels {
		got := out.Pixels[i]
		if in.Min() < 0 {
			m.InputNegativePixels++
		}
		if in.Max() > 1 {
			m.InputOverRangePixels++
		}
		if !in.InGamut() {
			m.InputOutOfGamut++
		}
		for _, ch := range [3]float64{in.R, in.G, in.B} {
			if ch < 0 {
				m.InputNegativeChans++
			}
			if ch > 1 {
				m.InputOverRangeChans++
			}
		}
		for _, ch := range [3]float64{got.R, got.G, got.B} {
			if ch == 0 || ch == 1 {
				m.OutputClippedChans++
			}
		}
		if !got.InGamut() {
			m.OutputOutOfGamut++
		}
		y := got.Luminance()
		m.MeanLuminance += y
		if y > m.MaxOutput {
			m.MaxOutput = y
		}
		if v := got.Max(); v > m.MaxOutput {
			m.MaxOutput = v
		}

		inHue, inChroma, okIn := hue(in)
		outHue, outChroma, okOut := hue(got)
		if !okIn || !okOut || inChroma < 1e-7 || outChroma < 1e-7 {
			continue
		}
		d := math.Abs(inHue - outHue)
		if d > 180 {
			d = 360 - d
		}
		shifts = append(shifts, d)
		hueSum += d * inChroma
		hueWeight += inChroma
	}

	m.ChromaticPixels = len(shifts)
	if len(src.Pixels) > 0 {
		m.MeanLuminance /= float64(len(src.Pixels))
	}
	if hueWeight > 0 {
		m.HueShiftMeanDeg = hueSum / hueWeight
	}
	if len(shifts) > 0 {
		sort.Float64s(shifts)
		m.HueShiftMaxDeg = shifts[len(shifts)-1]
		m.HueShiftP95Deg = percentile(shifts, 0.95)
	}
	return m
}

func percentile(sorted []float64, p float64) float64 {
	if len(sorted) == 0 {
		return 0
	}
	idx := int(math.Ceil(p*float64(len(sorted)))) - 1
	return sorted[clampIndex(idx, len(sorted))]
}

func clampIndex(i, n int) int {
	if i < 0 {
		return 0
	}
	if i >= n {
		return n - 1
	}
	return i
}

// hue returns the HSV hue in degrees and the chroma (max-min). The formula is
// valid for unclamped and negative components, which is important because the
// source is HDR.
func hue(c hdr.RGB) (float64, float64, bool) {
	maximum := c.Max()
	minimum := c.Min()
	chroma := maximum - minimum
	if !finite(maximum) || chroma <= 0 {
		return 0, 0, false
	}
	var h float64
	switch maximum {
	case c.R:
		h = 60 * math.Mod((c.G-c.B)/chroma, 6)
	case c.G:
		h = 60 * ((c.B-c.R)/chroma + 2)
	default:
		h = 60 * ((c.R-c.G)/chroma + 4)
	}
	if h < 0 {
		h += 360
	}
	return h, chroma, true
}

func finite(v float64) bool {
	return !math.IsNaN(v) && !math.IsInf(v, 0)
}
