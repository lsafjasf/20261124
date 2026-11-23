// Package fixtures builds deterministic HDR test scenes. The scenes exercise
// negative wide-gamut components, values far above one, and smooth and hard
// color edges without relying on external image files.
package fixtures

import (
	"math"

	"hdrthumb/hdr"
)

// Level is one of the three HDR intensity tiers used by the synthetic scenes.
type Level struct {
	Name string
	Gain float64
}

// Levels are deliberately spread across three orders of magnitude so all three
// compression presets have something meaningful to do.
var Levels = []Level{
	{Name: "low", Gain: 1.2},
	{Name: "medium", Gain: 10},
	{Name: "high", Gain: 80},
}

// Scene is a named synthetic frame.
type Scene struct {
	Name        string
	Description string
	Frame       hdr.Frame
}

// All returns the gradient, solid-color checkerboard, and emitter scenes at a
// common size.
func All(width, height int) []Scene {
	return []Scene{
		Gradient(width, height),
		ColorChecker(width, height),
		Emitters(width, height),
	}
}

// Gradient is a smooth two-axis ramp. The left side contains valid negative
// red components from a blue-violet wide-gamut color; the bottom spans roughly
// four stops of highlight intensity.
func Gradient(width, height int) Scene {
	f := hdr.NewFrame(width, height)
	left := hdr.RGB{R: -0.22, G: 0.05, B: 1.00}
	right := hdr.RGB{R: 1.00, G: 0.62, B: 0.34}
	for y := 0; y < height; y++ {
		v := normalized(y, height)
		exposure := 0.15 * math.Pow(128, v)
		for x := 0; x < width; x++ {
			u := normalized(x, width)
			base := mix(left, right, u)
			f.Set(x, y, scale(base, exposure))
		}
	}
	return Scene{
		Name:        "gradient",
		Description: "smooth wide-gamut/skin ramp with negative components and four stops of highlights",
		Frame:       f,
	}
}

// ColorChecker alternates solid color tiles. The three row groups use the three
// intensity levels, while the palette includes skin, blue-violet, magenta,
// cyan, neutral, and saturated primaries. Every tile is constant, so hue
// changes come only from the compressor, never from interpolation.
func ColorChecker(width, height int) Scene {
	f := hdr.NewFrame(width, height)
	palette := []hdr.RGB{
		{R: 1.35, G: 0.79, B: 0.52},
		{R: -0.14, G: 0.03, B: 1.55},
		{R: 1.70, G: -0.06, B: 1.35},
		{R: -0.09, G: 1.30, B: 1.60},
		{R: 2.10, G: 1.90, B: 1.70},
		{R: 0.08, G: 1.55, B: 0.16},
		{R: 2.30, G: 0.10, B: 0.05},
		{R: 0.22, G: 0.22, B: 0.22},
		{R: -0.22, G: 0.02, B: 0.95},
		{R: -0.07, G: 1.05, B: 1.15},
		{R: 1.80, G: 1.00, B: 0.20},
		{R: 0.02, G: 0.03, B: 0.04},
	}
	const columns = 6
	const rows = 6
	tileW := maxInt(1, width/columns)
	tileH := maxInt(1, height/rows)

	for y := 0; y < height; y++ {
		row := minInt(y/tileH, rows-1)
		level := Levels[minInt(row/2, len(Levels)-1)]
		for x := 0; x < width; x++ {
			col := minInt(x/tileW, columns-1)
			c := palette[(col+2*row)%len(palette)]
			f.Set(x, y, scale(c, level.Gain))
		}
	}
	return Scene{
		Name:        "color-checker",
		Description: "solid-color checkerboard with skin, blue-violet, magenta, and primary tiles at three intensities",
		Frame:       f,
	}
}

// Emitters places three radial glows side by side. Each glow has a warm core
// and a cool, slightly negative-red ring, reproducing the blue-violet edge that
// per-channel clamping turns black.
func Emitters(width, height int) Scene {
	f := hdr.NewFrame(width, height)
	sigma := math.Max(2, float64(height)*0.085)
	ringRadius := sigma * 2.5
	ringWidth := math.Max(1, sigma*0.25)

	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			c := hdr.RGB{R: 0.006, G: 0.007, B: 0.010}
			for i, level := range Levels {
				cx := float64(width) * (0.5 + float64(i-1)*0.33)
				cy := float64(height) * 0.5
				d := math.Hypot(float64(x)-cx, float64(y)-cy)
				core := math.Exp(-0.5*(d/sigma)*(d/sigma)) * level.Gain
				halo := math.Exp(-0.5*(d/(sigma*2.6))*(d/(sigma*2.6))) * level.Gain * 0.08
				ring := math.Exp(-0.5*((d-ringRadius)/ringWidth)*((d-ringRadius)/ringWidth)) * level.Gain * 0.15
				c.R += core*0.95 + halo*0.28 - ring*0.70
				c.G += core*0.56 + halo*0.48 + ring*0.03
				c.B += core*0.20 + halo*0.82 + ring*0.90
			}
			f.Set(x, y, c)
		}
	}
	return Scene{
		Name:        "emitters",
		Description: "three glowing bodies with warm cores and negative-red blue-violet rims",
		Frame:       f,
	}
}

func normalized(i, n int) float64 {
	if n <= 1 {
		return 0
	}
	return float64(i) / float64(n-1)
}

func mix(a, b hdr.RGB, t float64) hdr.RGB {
	return hdr.RGB{
		R: a.R + (b.R-a.R)*t,
		G: a.G + (b.G-a.G)*t,
		B: a.B + (b.B-a.B)*t,
	}
}

func scale(c hdr.RGB, gain float64) hdr.RGB {
	return hdr.RGB{R: c.R * gain, G: c.G * gain, B: c.B * gain}
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
