package main

import "math"

// RGB is a linear-light (scene-referred) RGB triple, not gamma encoded.
// Values are allowed to leave [0,1]; negatives and values above one carry
// the out-of-gamut information we need to compress.
type RGB struct {
	R, G, B float64
}

// XYZ is a CIE 1931 XYZ triple normalized to D65 (Y=1 at white).
type XYZ struct {
	X, Y, Z float64
}

// BT.2020 and BT.709 row-major matrices derived from the ITU primaries and
// the D65 white point. rgb2xyz maps linear RGB to XYZ; xyz2rgb is its inverse.
var (
	rgb2xyz2020 = [3][3]float64{
		{0.6369580483012914, 0.14461690358620832, 0.16888097516417205},
		{0.2627002120112671, 0.6779980715188708, 0.05930171646986196},
		{0.0000000000000000, 0.028072693049087428, 1.0609850577107901},
	}
	xyz2rgb2020 = [3][3]float64{
		{1.7166511879712674, -0.35567078377639233, -0.2533662813736598},
		{-0.6666843518324886, 1.6164812366355988, 0.01576854581391914},
		{0.01763985744531078, -0.04277061325780851, 0.9425397016095277},
	}
	rgb2xyz709 = [3][3]float64{
		{0.4123907992659595, 0.357584339383878, 0.1804807884018343},
		{0.2126390058715104, 0.715168678767756, 0.0721923154707338},
		{0.01933081871559185, 0.119194779794626, 0.9505321522496607},
	}
	xyz2rgb709 = [3][3]float64{
		{3.2409699419045226, -1.5373831775700941, -0.4986107602930034},
		{-0.9692436362808796, 1.8759675015077202, 0.0415550574071756},
		{0.05563007969699365, -0.2039769588889765, 1.0569715142428784},
	}

	// Rec.709 relative-luminance row of rgb2xyz709.
	luma709 = RGB{0.2126390058715104, 0.715168678767756, 0.0721923154707338}
	// Rec.2020 non-constant-luminance luma weights.
	luma2020 = RGB{0.2627, 0.6780, 0.0593}
)

func matMul(m [3][3]float64, c RGB) RGB {
	v := [3]float64{c.R, c.G, c.B}
	out := [3]float64{}
	for i := 0; i < 3; i++ {
		for j := 0; j < 3; j++ {
			out[i] += m[i][j] * v[j]
		}
	}
	return RGB{out[0], out[1], out[2]}
}

func (c RGB) toXYZ(m [3][3]float64) XYZ {
	o := matMul(m, c)
	return XYZ{o.R, o.G, o.B}
}

func (x XYZ) toRGB(m [3][3]float64) RGB {
	return matMul(m, RGB{x.X, x.Y, x.Z})
}

// Rec2020To709Linear is the fixed, colorimetric NCLT matrix path. The result
// is left unclipped on purpose; callers decide how to handle excursions.
func Rec2020To709Linear(c RGB) RGB {
	return c.toXYZ(rgb2xyz2020).toRGB(xyz2rgb709)
}

func (c RGB) luma(w RGB) float64 {
	return c.R*w.R + c.G*w.G + c.B*w.B
}

// Encode709 applies the BT.709 opto-electrical transfer function to a
// linear value, producing display code values in [0,1].
func Encode709(linear float64) float64 {
	if linear < 0 {
		return -encode709Positive(-linear)
	}
	return encode709Positive(linear)
}

func encode709Positive(linear float64) float64 {
	if linear < 0.018 {
		return 4.5 * linear
	}
	return 1.099*math.Pow(linear, 0.45) - 0.099
}

// Decode709 inverts Encode709.
func Decode709(encoded float64) float64 {
	if encoded < 0 {
		return -decode709Positive(-encoded)
	}
	return decode709Positive(encoded)
}

func decode709Positive(encoded float64) float64 {
	if encoded < 0.081 {
		return encoded / 4.5
	}
	return math.Pow((encoded+0.099)/1.099, 1.0/0.45)
}

func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}
