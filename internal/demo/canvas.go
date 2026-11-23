package demo

import (
	"math"

	"github.com/lsafjasf/20261124/internal/master"
)

type canvas struct {
	f *master.Frame
}

func newCanvas(width, height int, shot string, frame int, subtitle master.Rect) *canvas {
	return &canvas{f: &master.Frame{
		Width: width, Height: height, Shot: shot, FrameNumber: frame,
		Subtitle: subtitle, Pixels: make([]master.Vec3, width*height),
	}}
}

func (c *canvas) set(x, y int, v master.Vec3) {
	if x < 0 || y < 0 || x >= c.f.Width || y >= c.f.Height {
		return
	}
	c.f.Pixels[y*c.f.Width+x] = v
}

func (c *canvas) at(x, y int) master.Vec3 {
	if x < 0 || y < 0 || x >= c.f.Width || y >= c.f.Height {
		return master.Vec3{}
	}
	return c.f.Pixels[y*c.f.Width+x]
}

func (c *canvas) fill(top, bottom master.Vec3) {
	for y := 0; y < c.f.Height; y++ {
		t := float64(y) / float64(max(c.f.Height-1, 1))
		row := mix(top, bottom, t)
		for x := 0; x < c.f.Width; x++ {
			c.set(x, y, row)
		}
	}
}

func (c *canvas) rect(x, y, w, h int, v master.Vec3) {
	for py := y; py < y+h; py++ {
		for px := x; px < x+w; px++ {
			c.set(px, py, v)
		}
	}
}

func (c *canvas) rectMix(x, y, w, h int, v master.Vec3, alpha float64) {
	for py := y; py < y+h; py++ {
		for px := x; px < x+w; px++ {
			c.set(px, py, mix(c.at(px, py), v, alpha))
		}
	}
}

func (c *canvas) ellipse(cx, cy, rx, ry float64, shade func(x, y int, t float64) master.Vec3) {
	for y := int(cy - ry - 2); y <= int(cy+ry+2); y++ {
		for x := int(cx - rx - 2); x <= int(cx+rx+2); x++ {
			dx := (float64(x) + 0.5 - cx) / rx
			dy := (float64(y) + 0.5 - cy) / ry
			d := dx*dx + dy*dy
			if d > 1 {
				continue
			}
			alpha := clamp01((1-d)*12 + 0.15)
			c.set(x, y, mix(c.at(x, y), shade(x, y, 1-d), alpha))
		}
	}
}

func (c *canvas) circle(cx, cy, radius float64, shade func(t float64) master.Vec3) {
	c.ellipse(cx, cy, radius, radius, func(_ int, _ int, t float64) master.Vec3 {
		return shade(t)
	})
}

func mix(a, b master.Vec3, t float64) master.Vec3 {
	t = clamp01(t)
	return a.Add(b.Sub(a).Mul(t))
}

func scale(v master.Vec3, s float64) master.Vec3 {
	return v.Mul(s)
}

func clamp01(v float64) float64 {
	return math.Max(0, math.Min(1, v))
}

func smoothstep(edge0, edge1, x float64) float64 {
	t := clamp01((x - edge0) / (edge1 - edge0))
	return t * t * (3 - 2*t)
}

func gray18() master.Vec3 {
	return master.Vec3{master.MiddleGrayScene, master.MiddleGrayScene, master.MiddleGrayScene}
}
