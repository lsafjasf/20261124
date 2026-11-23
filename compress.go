package main

import "math"

// NegativeReport explains how negative channels were made non-negative.
type NegativeReport struct {
	Shift       float64 // constant added to every channel (desaturation step)
	Rescale     float64 // common gain restoring the original luma
	HadNegative bool
	LumaBefore  float64
	LumaAfter   float64
}

// liftNegatives removes negative components while preserving relative
// luminance. Step one adds -min(R,G,B) to every channel (a move toward the
// achromatic axis); step two rescales all channels so the weighted luma
// returns to its original value. Hue and the two positive channel ratios are
// untouched by the uniform gain.
func liftNegatives(c RGB, w RGB) (RGB, NegativeReport) {
	rep := NegativeReport{LumaBefore: c.luma(w)}
	mn := math.Min(c.R, math.Min(c.G, c.B))
	if mn >= 0 {
		rep.LumaAfter = rep.LumaBefore
		return c, rep
	}
	rep.HadNegative = true
	rep.Shift = -mn
	lifted := RGB{c.R + rep.Shift, c.G + rep.Shift, c.B + rep.Shift}
	target := rep.LumaBefore
	current := lifted.luma(w)
	if current > 0 {
		rep.Rescale = target / current
		lifted = RGB{lifted.R * rep.Rescale, lifted.G * rep.Rescale, lifted.B * rep.Rescale}
	}
	rep.LumaAfter = lifted.luma(w)
	return lifted, rep
}

// BoundaryReport explains the constant-luminance boundary search.
type BoundaryReport struct {
	Alpha         float64 // fraction of chroma retained; 1 means already in gamut
	Iterations    int
	HitFace       string // which [0,1] cube face stopped the ray
	LumaError     float64
	HeadroomScale float64 // applied when the whole achromatic line exceeds Y=1
}

const boundaryTolerance = 1e-10

// compressToBoundary keeps luminance and hue fixed and walks the color along
// its constant-luminance line toward the achromatic point until it reaches the
// Rec.709 RGB cube. alpha in [0,1] interpolates:
//
//	out = gray + alpha*(c-gray), gray = (Y,Y,Y), Y = dot(luma709, c)
//
// A binary search finds the largest feasible alpha, so the result lands on a
// gamut boundary face instead of being flattened by per-channel clipping.
func compressToBoundary(c RGB) (RGB, BoundaryReport) {
	rep := BoundaryReport{Alpha: 1, Iterations: 0, HitFace: "inside"}
	y := c.luma(luma709)

	scale := 1.0
	if y > 1 {
		// No point on the Y=constant achromatic line fits the cube; normalize
		// luminance first, then search at Y=1.
		scale = 1.0 / y
		c = RGB{c.R * scale, c.G * scale, c.B * scale}
		y = 1
	}
	if y <= 0 {
		// Non-positive luminance carries no reproducible chroma: anchor on
		// black while retaining any in-gamut positive signal rather than
		// collapsing the whole pixel to zero.
		anchored := RGB{clamp01(c.R), clamp01(c.G), clamp01(c.B)}
		rep.Alpha = 0
		rep.HitFace = "Y<=0 anchor"
		rep.HeadroomScale = scale
		rep.LumaError = math.Abs(anchored.luma(luma709) - c.luma(luma709))
		return anchored, rep
	}
	rep.HeadroomScale = scale

	gray := RGB{y, y, y}
	feasible := func(alpha float64) bool {
		for k := 0; k < 3; k++ {
			v := gray.channel(k) + alpha*(c.channel(k)-gray.channel(k))
			if v < -boundaryTolerance || v > 1+boundaryTolerance {
				return false
			}
		}
		return true
	}

	if feasible(1) {
		rep.LumaError = math.Abs(c.luma(luma709) - y/scale)
		return c, rep
	}

	lo, hi := 0.0, 1.0
	for i := 0; i < 64; i++ {
		mid := (lo + hi) / 2
		if feasible(mid) {
			lo = mid
		} else {
			hi = mid
		}
	}
	rep.Alpha = lo
	rep.Iterations = 64
	out := RGB{
		gray.R + lo*(c.R-gray.R),
		gray.G + lo*(c.G-gray.G),
		gray.B + lo*(c.B-gray.B),
	}
	rep.HitFace = boundaryFace(out)
	rep.LumaError = math.Abs(out.luma(luma709)-y) / math.Max(math.Abs(y), 1e-12)
	return out, rep
}

func (c RGB) channel(k int) float64 {
	switch k {
	case 0:
		return c.R
	case 1:
		return c.G
	default:
		return c.B
	}
}

func boundaryFace(c RGB) string {
	names := []string{"R", "G", "B"}
	for k := 0; k < 3; k++ {
		v := c.channel(k)
		if math.Abs(v) < 1e-6 {
			return names[k] + "=0"
		}
		if math.Abs(v-1) < 1e-6 {
			return names[k] + "=1"
		}
	}
	return "interior"
}
