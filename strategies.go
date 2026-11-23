package main

// StrategyName enumerates the three comparable gamut policies.
type StrategyName string

const (
	StrategyClip     StrategyName = "channel_clip"
	StrategyBoundary StrategyName = "boundary_search"
	StrategyMatrix   StrategyName = "fixed_matrix"
)

// PixelResult carries every intermediate needed for the evidence report.
type PixelResult struct {
	Index            int
	Source2020       RGB
	AfterLift2020    RGB
	Linear709Raw     RGB // colorimetric Rec.709 before any gamut policy
	OutputLinear709  RGB
	Display709       RGB // gamma encoded and clamped for PPM
	Neg              NegativeReport
	Boundary         BoundaryReport
	PreClipExcursion bool // fixed-matrix value left [0,1] before display clamp
}

// runClip is the naive per-channel truncation policy: apply the colorimetric
// matrix, then force every Rec.709 channel into [0,1] independently. The
// independent clamps change hue and relative luminance and flatten neon
// highlights onto a single cube face, which is the failure mode under test.
func runClip(src RGB) PixelResult {
	res := PixelResult{Source2020: src}
	res.AfterLift2020 = src
	out := Rec2020To709Linear(src)
	res.Linear709Raw = out
	res.OutputLinear709 = RGB{clamp01(out.R), clamp01(out.G), clamp01(out.B)}
	res.Display709 = encodeClamp(res.OutputLinear709)
	return res
}

// runBoundary is the explainable policy: negative lift in Rec.2020 first,
// colorimetric conversion, a second negative lift in Rec.709, then the
// constant-luminance hue-preserving boundary search.
func runBoundary(src RGB) PixelResult {
	res := PixelResult{Source2020: src}
	lifted2020, neg := liftNegatives(src, luma2020)
	res.AfterLift2020 = lifted2020
	res.Neg = neg
	raw := Rec2020To709Linear(lifted2020)
	res.Linear709Raw = raw
	lifted709, neg709 := liftNegatives(raw, luma709)
	if neg709.HadNegative {
		res.Neg.HadNegative = true
		res.Neg.Shift += neg709.Shift
	}
	out, bound := compressToBoundary(lifted709)
	res.Boundary = bound
	res.OutputLinear709 = out
	res.Display709 = encodeClamp(out)
	return res
}

// runMatrix is the pure fixed-matrix reference: no gamut-aware compression at
// all, only the Rec.2020->XYZ->Rec.709 conversion. Excursions are recorded and
// truncated solely so the value can be rendered to PPM.
func runMatrix(src RGB) PixelResult {
	res := PixelResult{Source2020: src}
	res.AfterLift2020 = src
	raw := Rec2020To709Linear(src)
	res.Linear709Raw = raw
	res.PreClipExcursion = raw.R < 0 || raw.R > 1 || raw.G < 0 || raw.G > 1 || raw.B < 0 || raw.B > 1
	res.OutputLinear709 = raw
	res.Display709 = encodeClamp(raw)
	return res
}

func encodeClamp(c RGB) RGB {
	return RGB{
		clamp01(Encode709(c.R)),
		clamp01(Encode709(c.G)),
		clamp01(Encode709(c.B)),
	}
}

// RunStrategy dispatches a source pixel through the named policy.
func RunStrategy(name StrategyName, src RGB) PixelResult {
	switch name {
	case StrategyClip:
		return runClip(src)
	case StrategyBoundary:
		return runBoundary(src)
	case StrategyMatrix:
		return runMatrix(src)
	default:
		panic("unknown strategy: " + name)
	}
}

var allStrategies = []StrategyName{StrategyClip, StrategyBoundary, StrategyMatrix}
