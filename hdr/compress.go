package hdr

import "math"

// Reason describes why a pixel could not be represented faithfully.
type Reason string

const (
	ReasonNonFinite         Reason = "non-finite"
	ReasonNegativeLuminance Reason = "negative-luminance"
	ReasonHighlightDetail   Reason = "highlight-detail-lost"
)

// PixelResult is the displayable color plus a description of any information
// that the compression could not preserve.
type PixelResult struct {
	// Color is always finite and inside [0,1]^3.
	Color RGB
	// NegativeRecovered is true when at least one component was below zero and
	// the whole triplet was desaturated toward its luminance instead of being
	// clamped channel by channel.
	NegativeRecovered bool
	// HighlightCompressed is true when the brightest component exceeded one and
	// passed through the shoulder.
	HighlightCompressed bool
	// Unrecoverable is the reason detail was lost, if any.
	Unrecoverable Reason
}

// Compress converts one linear-light HDR color into a displayable SDR color.
//
// The mapping has two distinct stages:
//
//  1. Wide-gamut recovery. Negative components are legitimate descriptors of
//     saturated colors outside the sRGB triangle. They are fixed by
//     desaturating the complete triplet toward its luminance. That preserves
//     both hue and luminance; clamping each component independently would not.
//  2. Highlight compression. The brightest component is passed through a
//     C1-continuous shoulder and the triplet is scaled by the same factor.
//     Chromaticity (and therefore hue) is preserved.
//
// The two stages are intentionally separate: a negative component is never
// mistaken for an overflow, and an overflow is never repaired by shifting a
// single channel.
func Compress(c RGB, p Preset) PixelResult {
	p = p.normalized()
	res := PixelResult{}

	if !c.IsFinite() {
		res.Unrecoverable = ReasonNonFinite
		return res
	}

	result := c

	// Stage 1: recover negative (wide-gamut) components without changing
	// luminance. Lerping every channel toward the same luminance scales the
	// channel differences uniformly, so hue is preserved.
	if result.Min() < 0 {
		y := result.Luminance()
		if y <= 0 {
			res.Unrecoverable = ReasonNegativeLuminance
			return res
		}
		desat := negativeDesaturation(result, y)
		if desat >= 1 {
			res.Unrecoverable = ReasonNegativeLuminance
			return res
		}
		// A tiny margin keeps round-off from leaving the recovered minimum
		// microscopically below zero.
		desat = math.Min(1, desat+1e-12)
		result = result.lerp(RGB{y, y, y}, desat)
		res.NegativeRecovered = desat > 0
	}

	// Stage 2: fold the highlight range into the displayable interval. The
	// shoulder starts below 1 so it is continuous at the knee; scaling by the
	// compressed maximum keeps all three component ratios intact.
	if m := result.Max(); m > p.Knee {
		compressed := shoulder(m, p.Knee)
		if compressed >= 1 {
			compressed = math.Nextafter(1, 0)
		}
		result = result.scale(compressed / m)
	}
	if c.Max() > 1 {
		res.HighlightCompressed = true
	}

	// Numerical guards only. Well-formed inputs cannot reach the first branch
	// except for a sub-ulp residue after gamut recovery.
	if result.Min() < 0 && result.Min() > -1e-9 {
		result.R = math.Max(result.R, 0)
		result.G = math.Max(result.G, 0)
		result.B = math.Max(result.B, 0)
	}
	if result.Min() < 0 || result.Max() > 1 || !result.IsFinite() {
		result = result.clamped01()
	}

	if m := c.Max(); m > p.DetailLimit {
		res.Unrecoverable = ReasonHighlightDetail
	}

	res.Color = result
	return res
}

// NaiveClamp is the old behavior, retained for comparison and regression
// tests. It clamps each channel independently and therefore distorts hue.
func NaiveClamp(c RGB) RGB {
	if !c.IsFinite() {
		return RGB{}
	}
	return c.clamped01()
}

func negativeDesaturation(c RGB, y float64) float64 {
	t := 0.0
	for _, ch := range []float64{c.R, c.G, c.B} {
		if ch < 0 {
			d := y - ch
			if d <= 0 {
				return 1
			}
			if need := -ch / d; need > t {
				t = need
			}
		}
	}
	return clamp(t, 0, 1)
}

// shoulder maps [knee, +Inf) to [knee, 1). It matches the identity in value
// and slope at the knee, then rolls off smoothly, so compressed highlights do
// not acquire a visible seam.
func shoulder(m, knee float64) float64 {
	if m <= knee {
		return m
	}
	span := 1 - knee
	x := (m - knee) / span
	if math.IsInf(x, 1) {
		return 1
	}
	return knee + (m-knee)/(1+x)
}
