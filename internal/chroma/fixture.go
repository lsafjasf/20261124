package chroma

import "fmt"

// Fixture contains a known 4:4:4 image, its derived luma/chroma planes, and
// the full-resolution RGB reference.
type Fixture struct {
	Name      string
	W         int
	H         int
	Original  *YUVImage
	Luma      *Plane
	Cb        *Plane
	Cr        *Plane
	Reference *RGBImage
}

func newFixture(name string, w, h int, sample func(x, y int) YCbCr) (*Fixture, error) {
	if w <= 0 || h <= 0 {
		return nil, fmt.Errorf("invalid fixture size %dx%d", w, h)
	}
	original := &YUVImage{W: w, H: h, Pix: make([]YCbCr, w*h)}
	luma := NewPlane(w, h)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			p := sample(x, y)
			original.Pix[y*w+x] = p
			luma.Set(x, y, p.Y)
		}
	}
	cb, cr, err := Subsample420(original)
	if err != nil {
		return nil, err
	}
	return &Fixture{
		Name:      name,
		W:         w,
		H:         h,
		Original:  original,
		Luma:      luma,
		Cb:        cb,
		Cr:        cr,
		Reference: YUVToRGB(original),
	}, nil
}

// EdgeFixture is a hard vertical blue/orange edge with a simultaneous
// luminance step. The default size is deliberately odd in both dimensions.
func EdgeFixture(w, h int) (*Fixture, error) {
	edgeX := (w + 1) / 2
	edgeY := (h + 1) / 2
	return newFixture("blue-orange-edge", w, h, func(x, y int) YCbCr {
		if x < edgeX {
			p := RGBToYCbCr(RGB{R: 255, G: 96, B: 16})
			if y >= edgeY {
				p = RGBToYCbCr(RGB{R: 240, G: 144, B: 32})
			}
			return p
		}
		p := RGBToYCbCr(RGB{R: 0, G: 72, B: 255})
		if y >= edgeY {
			p = RGBToYCbCr(RGB{R: 0, G: 128, B: 220})
		}
		return p
	})
}

// RampFixture uses a linear chroma ramp. Centered bilinear reconstruction is
// exact in the interior for a linear ramp, so any offset is visible directly.
func RampFixture(w, h int) (*Fixture, error) {
	return newFixture("linear-chroma-ramp", w, h, func(x, _ int) YCbCr {
		return YCbCr{
			Y:  128,
			Cb: 48 + 2*float64(x),
			Cr: 208 - 2*float64(x),
		}
	})
}
