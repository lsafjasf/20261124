package main

// Frame is one linear-light HDR RGB frame.
type Frame struct {
	W, H int
	Pix  []float64 // len = W*H*3, linear scene-referred RGB
}

func (f *Frame) Clone() *Frame {
	p := make([]float64, len(f.Pix))
	copy(p, f.Pix)
	return &Frame{W: f.W, H: f.H, Pix: p}
}

// Shot describes one captured shot. A "bright" shot is the same scene under
// a stronger key light: every linear value scales by KeyLight.
type Shot struct {
	Name     string
	KeyLight float64 // 1.0 = normal shot, 4.0 = bright shot
}

// SkinReflectance is a typical caucasian skin reflectance (linear RGB).
// The skin patch occupies a fixed rectangle so diagnostics can probe it.
var SkinReflectance = [3]float64{0.480, 0.318, 0.252}

const (
	SkinX0, SkinY0 = 8, 8
	SkinX1, SkinY1 = 23, 23
)

// MakeScene builds a 64x64 synthetic HDR frame:
//   - background: smooth vertical gradient 0.04..0.30 (linear)
//   - skin patch: SkinReflectance rectangle
//   - specular highlights: two bright discs up to KeyLight*WhitePoint
//
// All values are multiplied by KeyLight, so a bright shot is exactly the
// normal shot with more light — the correct compensation is pure exposure
// gain, which is what makes "not an exposure problem" testable.
func MakeScene(shot Shot) *Frame {
	const w, h = 64, 64
	f := &Frame{W: w, H: h, Pix: make([]float64, w*h*3)}
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			i := (y*w + x) * 3
			g := 0.04 + 0.26*float64(y)/float64(h-1)
			f.Pix[i+0] = g
			f.Pix[i+1] = g * 0.96
			f.Pix[i+2] = g * 0.90
		}
	}
	for y := SkinY0; y <= SkinY1; y++ {
		for x := SkinX0; x <= SkinX1; x++ {
			i := (y*w + x) * 3
			// slight vertical shading so the patch is not perfectly flat
			shade := 1.0 - 0.15*float64(y-SkinY0)/float64(SkinY1-SkinY0)
			f.Pix[i+0] = SkinReflectance[0] * shade
			f.Pix[i+1] = SkinReflectance[1] * shade
			f.Pix[i+2] = SkinReflectance[2] * shade
		}
	}
	paintDisc(f, 44, 16, 6, [3]float64{WhitePoint, WhitePoint, WhitePoint})
	paintDisc(f, 48, 44, 5, [3]float64{0.9 * WhitePoint, 0.8 * WhitePoint, 0.7 * WhitePoint})
	for i := range f.Pix {
		f.Pix[i] *= shot.KeyLight
	}
	return f
}

func paintDisc(f *Frame, cx, cy, r int, rgb [3]float64) {
	for y := cy - r; y <= cy+r; y++ {
		for x := cx - r; x <= cx+r; x++ {
			if x < 0 || y < 0 || x >= f.W || y >= f.H {
				continue
			}
			dx, dy := x-cx, y-cy
			if dx*dx+dy*dy > r*r {
				continue
			}
			i := (y*f.W + x) * 3
			f.Pix[i+0], f.Pix[i+1], f.Pix[i+2] = rgb[0], rgb[1], rgb[2]
		}
	}
}

// SkinPatchMean returns the mean RGB over the skin rectangle.
func SkinPatchMean(f *Frame) [3]float64 {
	var m [3]float64
	n := 0.0
	for y := SkinY0; y <= SkinY1; y++ {
		for x := SkinX0; x <= SkinX1; x++ {
			i := (y*f.W + x) * 3
			m[0] += f.Pix[i+0]
			m[1] += f.Pix[i+1]
			m[2] += f.Pix[i+2]
			n++
		}
	}
	return [3]float64{m[0] / n, m[1] / n, m[2] / n}
}

// Chroma is a simple colorfulness measure: (max-min)/max of RGB.
// Gray skin has chroma near 0; healthy skin sits around 0.3-0.5.
func Chroma(rgb [3]float64) float64 {
	mx, mn := rgb[0], rgb[0]
	for _, v := range rgb[1:] {
		if v > mx {
			mx = v
		}
		if v < mn {
			mn = v
		}
	}
	if mx <= 0 {
		return 0
	}
	return (mx - mn) / mx
}
