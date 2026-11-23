package hdr

import "math"

// RGB is a linear-light color. Components are deliberately not clamped: HDR
// sources can contain values above 1, and wide-gamut conversions can produce
// small negative components that are still valid colors.
type RGB struct {
	R float64
	G float64
	B float64
}

// Luminance returns linear sRGB luminance (Rec. 709 coefficients).
func (c RGB) Luminance() float64 {
	return 0.2126*c.R + 0.7152*c.G + 0.0722*c.B
}

func (c RGB) Min() float64 {
	return math.Min(c.R, math.Min(c.G, c.B))
}

func (c RGB) Max() float64 {
	return math.Max(c.R, math.Max(c.G, c.B))
}

func (c RGB) IsFinite() bool {
	return !math.IsNaN(c.R) && !math.IsInf(c.R, 0) &&
		!math.IsNaN(c.G) && !math.IsInf(c.G, 0) &&
		!math.IsNaN(c.B) && !math.IsInf(c.B, 0)
}

func (c RGB) InGamut() bool {
	return c.R >= 0 && c.R <= 1 &&
		c.G >= 0 && c.G <= 1 &&
		c.B >= 0 && c.B <= 1
}

func (c RGB) clamped01() RGB {
	return RGB{clamp01(c.R), clamp01(c.G), clamp01(c.B)}
}

func (c RGB) lerp(to RGB, t float64) RGB {
	return RGB{
		R: c.R + (to.R-c.R)*t,
		G: c.G + (to.G-c.G)*t,
		B: c.B + (to.B-c.B)*t,
	}
}

func (c RGB) scale(f float64) RGB {
	return RGB{c.R * f, c.G * f, c.B * f}
}

func clamp01(v float64) float64 {
	if math.IsNaN(v) {
		return 0
	}
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

func clamp(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
