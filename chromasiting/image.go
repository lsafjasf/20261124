package chromasiting

import "math"

// Plane is a float64 single-component raster, row-major.
type Plane struct {
	W, H int
	Pix  []float64
}

func NewPlane(w, h int) Plane {
	return Plane{W: w, H: h, Pix: make([]float64, w*h)}
}

func (p Plane) At(x, y int) float64 {
	return p.Pix[y*p.W+x]
}

func (p Plane) Set(x, y int, v float64) {
	p.Pix[y*p.W+x] = v
}

func (p Plane) Row(y int) []float64 {
	return p.Pix[y*p.W : (y+1)*p.W]
}

// sample bilinearly interpolates p at fractional coordinate (fx, fy),
// clamping to the plane edges.
func (p Plane) sample(fx, fy float64) float64 {
	fx = math.Max(0, math.Min(fx, float64(p.W-1)))
	fy = math.Max(0, math.Min(fy, float64(p.H-1)))
	x0 := int(math.Floor(fx))
	y0 := int(math.Floor(fy))
	x1 := min(x0+1, p.W-1)
	y1 := min(y0+1, p.H-1)
	dx := fx - float64(x0)
	dy := fy - float64(y0)
	top := p.At(x0, y0)*(1-dx) + p.At(x1, y0)*dx
	bot := p.At(x0, y1)*(1-dx) + p.At(x1, y1)*dx
	return top*(1-dy) + bot*dy
}

// Image444 is a full-resolution YCbCr image.
type Image444 struct {
	Y, Cb, Cr Plane
}

// Image420 is a 4:2:0 subsampled image. Chroma planes are
// ceil(W/2) x ceil(H/2), so odd luma sizes are representable.
type Image420 struct {
	Y      Plane
	Cb, Cr Plane
}

// Subsample420 converts 4:4:4 to 4:2:0 by evaluating the full-resolution
// chroma signal at each siting point (2*cx+ox, 2*cy+oy). Real encoders use
// dedicated filters; point evaluation keeps the siting phase explicit.
func Subsample420(img Image444, s Siting) Image420 {
	ox, oy := s.Offset()
	w, h := img.Y.W, img.Y.H
	cw, ch := (w+1)/2, (h+1)/2
	out := Image420{Y: img.Y, Cb: NewPlane(cw, ch), Cr: NewPlane(cw, ch)}
	for cy := 0; cy < ch; cy++ {
		for cx := 0; cx < cw; cx++ {
			lx := 2*float64(cx) + ox
			ly := 2*float64(cy) + oy
			out.Cb.Set(cx, cy, img.Cb.sample(lx, ly))
			out.Cr.Set(cx, cy, img.Cr.sample(lx, ly))
		}
	}
	return out
}

// Upsample444 reconstructs 4:4:4 chroma from 4:2:0 using bilinear
// interpolation positioned by the siting: luma pixel (x, y) reads chroma
// coordinate ((x-ox)/2, (y-oy)/2).
func Upsample444(img Image420, s Siting) Image444 {
	ox, oy := s.Offset()
	w, h := img.Y.W, img.Y.H
	out := Image444{Y: img.Y, Cb: NewPlane(w, h), Cr: NewPlane(w, h)}
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			cx := (float64(x) - ox) / 2
			cy := (float64(y) - oy) / 2
			out.Cb.Set(x, y, img.Cb.sample(cx, cy))
			out.Cr.Set(x, y, img.Cr.sample(cx, cy))
		}
	}
	return out
}
