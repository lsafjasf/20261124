package main

import "math"

// D65 reference white used by both BT.2020 and BT.709.
const (
	whiteX = 0.95047
	whiteY = 1.0
	whiteZ = 1.08883
)

// Lab is a CIELAB L*a*b* triple.
type Lab struct {
	L, A, B float64
}

var labDelta = 6.0 / 29.0

func labF(t float64) float64 {
	if t > labDelta*labDelta*labDelta {
		return math.Cbrt(t)
	}
	return t/(3*labDelta*labDelta) + 4.0/29.0
}

// XYZToLab converts D65 XYZ to CIELAB. Cube roots remain defined for slightly
// negative scene-referred components, which keeps out-of-gamut sources
// comparable against their in-gamut reproductions.
func XYZToLab(x XYZ) Lab {
	fx := labF(x.X / whiteX)
	fy := labF(x.Y / whiteY)
	fz := labF(x.Z / whiteZ)
	return Lab{
		L: 1.16*fy - 0.16,
		A: 5.0 * (fx - fy),
		B: 2.0 * (fy - fz),
	}
}

func deg(rad float64) float64 { return rad * 180 / math.Pi }
func rad(d float64) float64   { return d * math.Pi / 180 }

// DeltaE2000 implements the CIE 2000 color-difference formula with unit
// parametric weights, following the Sharma/Wu/Dalal reference procedure.
func DeltaE2000(p, q Lab) float64 {
	c1 := math.Hypot(p.A, p.B)
	c2 := math.Hypot(q.A, q.B)
	cBar := (c1 + c2) / 2
	cBar7 := math.Pow(cBar, 7)
	g := 0.5 * (1 - math.Sqrt(cBar7/(cBar7+math.Pow(25, 7))))

	a1 := (1 + g) * p.A
	a2 := (1 + g) * q.A
	cc1 := math.Hypot(a1, p.B)
	cc2 := math.Hypot(a2, q.B)

	h1 := hueDegrees(p.B, a1, cc1)
	h2 := hueDegrees(q.B, a2, cc2)

	dL := q.L - p.L
	dC := cc2 - cc1
	dh := 0.0
	if cc1 != 0 && cc2 != 0 {
		dh = h2 - h1
		if dh > 180 {
			dh -= 360
		} else if dh < -180 {
			dh += 360
		}
	}
	dH := 2 * math.Sqrt(cc1*cc2) * math.Sin(rad(dh)/2)

	lBar := (p.L + q.L) / 2
	cBarP := (cc1 + cc2) / 2
	hBar := 0.0
	if cc1 != 0 && cc2 != 0 {
		if math.Abs(h1-h2) <= 180 {
			hBar = (h1 + h2) / 2
		} else if h1+h2 < 360 {
			hBar = (h1 + h2 + 360) / 2
		} else {
			hBar = (h1 + h2 - 360) / 2
		}
	}

	t := 1 - 0.17*math.Cos(rad(hBar-30)) +
		0.24*math.Cos(rad(2*hBar)) +
		0.32*math.Cos(rad(3*hBar+6)) -
		0.20*math.Cos(rad(4*hBar-63))
	dTheta := 30 * math.Exp(-math.Pow((hBar-275)/25, 2))
	cBarP7 := math.Pow(cBarP, 7)
	rc := 2 * math.Sqrt(cBarP7/(cBarP7+math.Pow(25, 7)))
	sl := 1 + 0.015*math.Pow(lBar-50, 2)/math.Sqrt(20+math.Pow(lBar-50, 2))
	sc := 1 + 0.045*cBarP
	sh := 1 + 0.015*cBarP*t
	rt := -math.Sin(rad(2*dTheta)) * rc

	lTerm := dL / sl
	cTerm := dC / sc
	hTerm := dH / sh
	return math.Sqrt(lTerm*lTerm + cTerm*cTerm + hTerm*hTerm + rt*cTerm*hTerm)
}

func hueDegrees(b, a, chroma float64) float64 {
	if chroma == 0 {
		return 0
	}
	h := deg(math.Atan2(b, a))
	if h < 0 {
		h += 360
	}
	return h
}

// HueFlip describes a hue rotation between a source and reproduction.
type HueFlip struct {
	Index      int
	SourceHue  float64
	OutputHue  float64
	Difference float64 // shortest circular angle in degrees
}

const neutralChromaGuard = 0.5

// hueFlip returns nil unless both colors are chromatic and their circular
// hue difference exceeds 90 degrees, which indicates a hue family change
// (e.g. magenta collapsing toward blue) rather than a small rotation.
func hueFlip(index int, src, out Lab) *HueFlip {
	cs := math.Hypot(src.A, src.B)
	co := math.Hypot(out.A, out.B)
	if cs < neutralChromaGuard || co < neutralChromaGuard {
		return nil
	}
	hs := labHue(src)
	ho := labHue(out)
	d := circularDistance(hs, ho)
	if d <= 90 {
		return nil
	}
	return &HueFlip{Index: index, SourceHue: hs, OutputHue: ho, Difference: d}
}

// hueRotation returns the shortest circular hue-angle distance in degrees,
// or zero when either side is near-neutral.
func hueRotation(src, out Lab) float64 {
	if math.Hypot(src.A, src.B) < neutralChromaGuard ||
		math.Hypot(out.A, out.B) < neutralChromaGuard {
		return 0
	}
	return circularDistance(labHue(src), labHue(out))
}

func labHue(c Lab) float64 {
	h := deg(math.Atan2(c.B, c.A))
	if h < 0 {
		h += 360
	}
	return h
}

func circularDistance(a, b float64) float64 {
	d := math.Abs(a - b)
	if d > 180 {
		d = 360 - d
	}
	return d
}
