package chroma

import "math"

// Convention controls where a 4:2:0 chroma sample is sited relative to luma.
type Convention uint8

const (
	// Centered sites each chroma sample at the center of its 2x2 luma block.
	// This is the correct mapping for centered-sited 4:2:0 (for example JPEG).
	Centered Convention = iota
	// LeftTop sites each chroma sample on the top-left luma sample. Treating
	// this as equivalent to Centered introduces a half-luma-pixel offset.
	LeftTop
)

func (c Convention) String() string {
	if c == Centered {
		return "centered"
	}
	return "left-top"
}

// ChromaCoordinate maps a destination luma coordinate to chroma sample-grid
// coordinates. A chroma coordinate q places the reconstructed point at
// 2*q+1 in luma coordinates.
func (c Convention) ChromaCoordinate(dest int) float64 {
	d := float64(dest)
	if c == Centered {
		return d/2 - 0.25
	}
	return d / 2
}

// Tap is one axis of a bilinear lookup. The two weights sum to one. At image
// boundaries both indices may refer to the same edge sample.
type Tap struct {
	I0 int     `json:"i0"`
	I1 int     `json:"i1"`
	W0 float64 `json:"w0"`
	W1 float64 `json:"w1"`
}

// AxisMapping describes one axis of a chroma lookup.
type AxisMapping struct {
	Dest       int     `json:"dest"`
	Coordinate float64 `json:"coordinate"`
	Tap
}

// Axis maps one destination coordinate to two clamped source samples.
func Axis(dest, sourceLen int, conv Convention) AxisMapping {
	q := conv.ChromaCoordinate(dest)
	base := int(math.Floor(q))
	frac := q - float64(base)
	i0, i1 := base, base+1
	w0, w1 := 1-frac, frac

	left := clampIndex(i0, sourceLen)
	right := clampIndex(i1, sourceLen)
	if left == right {
		w0, w1 = 1, 0
	}
	return AxisMapping{
		Dest:       dest,
		Coordinate: q,
		Tap: Tap{
			I0: left,
			I1: right,
			W0: w0,
			W1: w1,
		},
	}
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

// PixelMapping is the complete coordinate and weight record for one output
// pixel. X and Y coordinates are in chroma-sample units. LumaPosition is the
// reconstructed point in luma-pixel units.
type PixelMapping struct {
	X            int
	Y            int
	XAxis        AxisMapping
	YAxis        AxisMapping
	LumaPosition struct {
		X float64
		Y float64
	}
}

// MapPixel returns the coordinate and bilinear weights for one output pixel.
func MapPixel(x, y int, planeW, planeH int, conv Convention) PixelMapping {
	xm := Axis(x, planeW, conv)
	ym := Axis(y, planeH, conv)
	m := PixelMapping{X: x, Y: y, XAxis: xm, YAxis: ym}
	m.LumaPosition.X = 2*xm.Coordinate + 1
	m.LumaPosition.Y = 2*ym.Coordinate + 1
	return m
}
