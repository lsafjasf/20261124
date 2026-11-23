// Package buggy reproduces the three historical defects reported by the
// color department, one per pipeline stage, so the error report can
// localize each fault against a correct reference:
//
//   - Stage 1 (input interpretation): DoubleGamma skips the sRGB EOTF
//     and treats code values as linear light, so the Rec.709 OETF is
//     applied on top of the sRGB OETF — gamma applied twice, lifting
//     and graying the shadows.
//   - Stage 2 (ACEScg intermediate): ClipNegatives clamps the
//     scene-linear ACEScg triplet to [0,1], destroying legal negative
//     values produced by wide-gamut content.
//   - Stage 3 (8/16-bit path): TruncateDepth runs the 16-bit path at
//     8-bit precision (high byte only), causing banding in smooth dark
//     gradients.
//
// None of these may be "fixed" with exposure compensation; they are
// transfer-function and data-path errors.
package buggy

import (
	"math"

	c "github.com/lsafjasf/20261124/colorxform"
)

func clamp01(v float64) float64 {
	return math.Min(math.Max(v, 0), 1)
}

// DoubleGamma reproduces the stage-1 defect: the sRGB EOTF is skipped
// and the non-linear input is treated as linear light.
func DoubleGamma(r, g, b float64) (float64, float64, float64) {
	ar, ag, ab := c.SRGBToACEScg.Apply(r, g, b) // no SRGBDecode
	rr, rg, rb := c.ACEScgToSRGB.Apply(ar, ag, ab)
	return c.Rec709Encode(rr), c.Rec709Encode(rg), c.Rec709Encode(rb)
}

// ClipNegatives reproduces the stage-2 defect: legal negative ACEScg
// values are clamped away in the intermediate domain.
func ClipNegatives(r, g, b float64) (float64, float64, float64) {
	lr, lg, lb := c.SRGBDecode(r), c.SRGBDecode(g), c.SRGBDecode(b)
	ar, ag, ab := c.SRGBToACEScg.Apply(lr, lg, lb)
	return ClipNegativesFromACEScg(ar, ag, ab)
}

// ClipNegativesFromACEScg is the stage-2 defect applied to content
// already in the ACEScg domain (e.g. wide-gamut renders), where legal
// negative values actually occur.
func ClipNegativesFromACEScg(ar, ag, ab float64) (float64, float64, float64) {
	ar, ag, ab = clamp01(ar), clamp01(ag), clamp01(ab)
	rr, rg, rb := c.ACEScgToSRGB.Apply(ar, ag, ab)
	return c.Rec709Encode(rr), c.Rec709Encode(rg), c.Rec709Encode(rb)
}

// TruncateDepth reproduces the stage-3 defect: the 16-bit path keeps
// only the high byte of each sample before conversion.
func TruncateDepth(r, g, b float64) (float64, float64, float64) {
	tr := func(v float64) float64 {
		return math.Floor(v*65535/256) * 256 / 65535
	}
	lr, lg, lb := c.SRGBDecode(tr(r)), c.SRGBDecode(tr(g)), c.SRGBDecode(tr(b))
	ar, ag, ab := c.SRGBToACEScg.Apply(lr, lg, lb)
	rr, rg, rb := c.ACEScgToSRGB.Apply(ar, ag, ab)
	return c.Rec709Encode(rr), c.Rec709Encode(rg), c.Rec709Encode(rb)
}

// Combined applies all three defects at once (the historical "before"
// build).
func Combined(r, g, b float64) (float64, float64, float64) {
	tr := func(v float64) float64 {
		return math.Floor(v*65535/256) * 256 / 65535
	}
	ar, ag, ab := c.SRGBToACEScg.Apply(tr(r), tr(g), tr(b)) // no EOTF
	ar, ag, ab = clamp01(ar), clamp01(ag), clamp01(ab)
	rr, rg, rb := c.ACEScgToSRGB.Apply(ar, ag, ab)
	return c.Rec709Encode(rr), c.Rec709Encode(rg), c.Rec709Encode(rb)
}
