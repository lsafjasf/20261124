package main

import "math"

const (
	imageWidth  = 120
	imageHeight = 80
	sweepRows   = 64
)

// neonSwatches are hand-picked scene-referred Rec.2020 linear colors. Values
// below zero and above one are intentional: they encode saturated neon and
// blue-violet highlights that cannot survive a Rec.709 window untouched.
var neonSwatches = []RGB{
	{0.55, 0.10, 1.35},   // neon violet
	{0.05, 0.30, 1.45},   // electric blue
	{1.25, 0.02, 1.05},   // neon magenta
	{-0.15, 1.15, 1.30},  // cyan with negative red
	{1.40, 0.05, 0.55},   // hot pink
	{0.35, -0.08, 0.95},  // deep purple, negative green
	{0.50, 1.45, -0.10},  // neon lime
	{1.35, 0.55, -0.20},  // amber neon
	{0.10, -0.12, 1.20},  // blue ink, negative green
	{1.50, 1.45, 1.55},   // white-hot specular
	{-2.00, -0.25, 0.25}, // deep-cyan signal beyond the gamut, flips under clip
	{-1.75, -0.50, 0.50}, // second extreme cyan-magenta stress pixel
}

// Synthesize builds the deterministic stress image and returns both the
// linear Rec.2020 source pixels and a directly clamped preview.
func Synthesize() (*Image, []RGB) {
	im := newImage(imageWidth, imageHeight)
	source := make([]RGB, imageWidth*imageHeight)

	for y := 0; y < sweepRows; y++ {
		v := float64(y) / float64(sweepRows-1)
		lumaLevel := 0.25 + 0.55*v
		saturation := 0.25 + 2.15*v
		for x := 0; x < imageWidth; x++ {
			h := 2 * math.Pi * float64(x) / float64(imageWidth)
			direction := RGB{
				math.Cos(h),
				math.Cos(h - 2*math.Pi/3),
				math.Cos(h + 2*math.Pi/3),
			}
			c := RGB{
				lumaLevel + saturation*direction.R,
				lumaLevel + saturation*direction.G,
				lumaLevel + saturation*direction.B,
			}
			idx := y*imageWidth + x
			source[idx] = c
			im.Pixels[idx] = encodeClamp(c)
		}
	}

	blockWidth := imageWidth / len(neonSwatches)
	for b, c := range neonSwatches {
		for y := sweepRows; y < imageHeight; y++ {
			for x := b * blockWidth; x < (b+1)*blockWidth && x < imageWidth; x++ {
				idx := y*imageWidth + x
				source[idx] = c
				im.Pixels[idx] = encodeClamp(c)
			}
		}
	}
	return im, source
}

// outOfGamutMask marks sources that already leave the Rec.2020 [0,1] cube or
// whose colorimetric Rec.709 conversion leaves the Rec.709 cube.
func outOfGamutMask(source []RGB) []bool {
	mark := make([]bool, len(source))
	for i, c := range source {
		oog := c.R < 0 || c.R > 1 || c.G < 0 || c.G > 1 || c.B < 0 || c.B > 1
		if !oog {
			r := Rec2020To709Linear(c)
			oog = r.R < 0 || r.R > 1 || r.G < 0 || r.G > 1 || r.B < 0 || r.B > 1
		}
		mark[i] = oog
	}
	return mark
}
