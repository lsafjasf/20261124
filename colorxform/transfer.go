// Package colorxform implements a scene-referred color pipeline:
// sRGB input -> linear sRGB -> ACEScg working space -> Rec.709 output.
//
// All transfer functions use mirrored (odd-symmetric) extensions for
// negative inputs so that legal out-of-gamut negative values produced
// by gamut matrices survive the round trip instead of being clipped.
package colorxform

import "math"

// SRGBDecode converts a non-linear sRGB code value to linear light.
// Values below 0 (and above 1) are handled by mirroring the curve,
// per the extended sRGB convention used in compositing pipelines.
func SRGBDecode(c float64) float64 {
	if c < 0 {
		return -SRGBDecode(-c)
	}
	if c <= 0.04045 {
		return c / 12.92
	}
	return math.Pow((c+0.055)/1.055, 2.4)
}

// SRGBEncode converts a linear-light sRGB value to a non-linear code
// value. Negative inputs are mirrored, never clipped.
func SRGBEncode(l float64) float64 {
	if l < 0 {
		return -SRGBEncode(-l)
	}
	if l <= 0.0031308 {
		return 12.92 * l
	}
	return 1.055*math.Pow(l, 1.0/2.4) - 0.055
}

// Rec709Encode applies the BT.709 OETF to a linear-light value.
// Negative inputs are mirrored, never clipped.
func Rec709Encode(l float64) float64 {
	if l < 0 {
		return -Rec709Encode(-l)
	}
	if l < 0.018 {
		return 4.5 * l
	}
	return 1.099*math.Pow(l, 0.45) - 0.099
}

// Rec709Decode inverts the BT.709 OETF. Negative inputs are mirrored.
func Rec709Decode(c float64) float64 {
	if c < 0 {
		return -Rec709Decode(-c)
	}
	if c < 4.5*0.018 {
		return c / 4.5
	}
	return math.Pow((c+0.099)/1.099, 1.0/0.45)
}
