package chroma

// RGB stores display-referred 8-bit component values as floats during the
// experiment. Values are clamped to [0, 255] only when an image is encoded.
type RGB struct {
	R float64
	G float64
	B float64
}

// YCbCr is a full-resolution sample in the BT.601 JPEG color space.
type YCbCr struct {
	Y  float64
	Cb float64
	Cr float64
}

// Plane is a single-channel, row-major plane.
type Plane struct {
	W   int
	H   int
	Pix []float64
}

// NewPlane returns a zero-filled plane.
func NewPlane(w, h int) *Plane {
	return &Plane{W: w, H: h, Pix: make([]float64, w*h)}
}

// At returns the sample at x, y. The caller must keep x and y in bounds.
func (p *Plane) At(x, y int) float64 {
	return p.Pix[y*p.W+x]
}

// Set stores a sample at x, y. The caller must keep x and y in bounds.
func (p *Plane) Set(x, y int, v float64) {
	p.Pix[y*p.W+x] = v
}

// YUVImage is a full-resolution 4:4:4 image.
type YUVImage struct {
	W   int
	H   int
	Pix []YCbCr
}

// At returns the sample at x, y.
func (im *YUVImage) At(x, y int) YCbCr {
	return im.Pix[y*im.W+x]
}

// RGBImage is a full-resolution RGB image.
type RGBImage struct {
	W   int
	H   int
	Pix []RGB
}

// At returns the sample at x, y.
func (im *RGBImage) At(x, y int) RGB {
	return im.Pix[y*im.W+x]
}
