package main

import (
	"image"
	"image/color"
	"math"
)

// canvas wraps image.RGBA with tiny drawing helpers (stdlib only).
type canvas struct{ *image.RGBA }

func newCanvas(w, h int, bg color.RGBA) *canvas {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	c := &canvas{img}
	c.fillRect(0, 0, w, h, bg)
	return c
}

func (c *canvas) fillRect(x, y, w, h int, col color.RGBA) {
	for j := y; j < y+h; j++ {
		for i := x; i < x+w; i++ {
			if i >= 0 && j >= 0 && i < c.Bounds().Dx() && j < c.Bounds().Dy() {
				c.SetRGBA(i, j, col)
			}
		}
	}
}

// blendRect alpha-blends col over the rectangle.
func (c *canvas) blendRect(x, y, w, h int, col color.RGBA, alpha float64) {
	for j := y; j < y+h; j++ {
		for i := x; i < x+w; i++ {
			if i < 0 || j < 0 || i >= c.Bounds().Dx() || j >= c.Bounds().Dy() {
				continue
			}
			old := c.RGBAAt(i, j)
			c.SetRGBA(i, j, color.RGBA{
				R: uint8(float64(old.R)*(1-alpha) + float64(col.R)*alpha),
				G: uint8(float64(old.G)*(1-alpha) + float64(col.G)*alpha),
				B: uint8(float64(old.B)*(1-alpha) + float64(col.B)*alpha),
				A: 255,
			})
		}
	}
}

func (c *canvas) vline(x, y0, y1 int, col color.RGBA) { c.fillRect(x, y0, 1, y1-y0, col) }
func (c *canvas) hline(x0, x1, y int, col color.RGBA) { c.fillRect(x0, y, x1-x0, 1, col) }

func (c *canvas) dot(cx, cy, r int, col color.RGBA) {
	c.fillRect(cx-r, cy-r, 2*r+1, 2*r+1, col)
}

var (
	colBg     = color.RGBA{18, 20, 26, 255}
	colGrid   = color.RGBA{60, 66, 78, 255}
	colAxis   = color.RGBA{120, 128, 140, 255}
	colText   = color.RGBA{210, 214, 220, 255}
	colDim    = color.RGBA{140, 146, 156, 255}
	colCurve  = color.RGBA{240, 240, 245, 255}
	colSample = color.RGBA{80, 220, 250, 255}
	colMid    = color.RGBA{250, 250, 250, 255}
	colRed    = color.RGBA{225, 45, 45, 255}
	colOrange = color.RGBA{240, 160, 40, 255}
	colGreen  = color.RGBA{80, 210, 110, 255}
)

var peakColors = []color.RGBA{colRed, colOrange, colGreen}

// Curve plot geometry: x = log10(nits) in [logMin, logMax], y = PQ [0,1].
const (
	plotX0, plotX1 = 90, 1200
	plotY0, plotY1 = 90, 450
	logMin, logMax = -2.0, 4.0
)

func xOfNits(nits float64) int {
	t := (math.Log10(nits) - logMin) / (logMax - logMin)
	return plotX0 + int(t*float64(plotX1-plotX0))
}

func nitsOfX(x int) float64 {
	t := float64(x-plotX0) / float64(plotX1-plotX0)
	return math.Pow(10, logMin+t*(logMax-logMin))
}

func yOfPQ(pq float64) int {
	return plotY1 - int(pq*float64(plotY1-plotY0))
}

// makeSamples returns n log-spaced luminance samples across the PQ range.
func makeSamples(n int) []float64 {
	out := make([]float64, n)
	for i := range out {
		out[i] = math.Pow(10, logMin+float64(i)/float64(n-1)*(logMax-logMin))
	}
	return out
}
