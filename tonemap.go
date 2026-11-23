package main

import "math"

// WhitePoint is the scene-referred luminance (in linear exposure units)
// that the tone mapper maps to display white 1.0.
const WhitePoint = 4.0

// ToneMap is a Reinhard-style filmic curve extended with a white point:
//
//	y = x * (1 + x/w^2) / (1 + x)
//
// It is monotonic on x >= 0, maps 0 -> 0 and WhitePoint -> 1.0, and has a
// closed-form inverse, which the diagnostics use to back-solve curves from
// intermediate pixel values.
func ToneMap(x float64) float64 {
	if x <= 0 {
		return 0
	}
	w2 := WhitePoint * WhitePoint
	return x * (1 + x/w2) / (1 + x)
}

// ToneMapInverse solves ToneMap(x) == y for x >= 0.
// y*(1+x) = x + x^2/w^2  =>  x^2/w^2 + (1-y)*x - y = 0
func ToneMapInverse(y float64) float64 {
	if y <= 0 {
		return 0
	}
	w2 := WhitePoint * WhitePoint
	// x = w^2/2 * ( (y-1) + sqrt((1-y)^2 + 4y/w^2) )
	disc := (1-y)*(1-y) + 4*y/w2
	return w2 / 2 * ((y - 1) + math.Sqrt(disc))
}

// SRGBEncode converts linear [0,1] to sRGB transfer-encoded [0,1].
func SRGBEncode(x float64) float64 {
	if x <= 0 {
		return 0
	}
	if x >= 1 {
		return 1
	}
	if x <= 0.0031308 {
		return 12.92 * x
	}
	return 1.055*math.Pow(x, 1.0/2.4) - 0.055
}

// SRGBDecode is the inverse of SRGBEncode, used by diagnostics to bring
// 8-bit output back to linear display-referred values.
func SRGBDecode(y float64) float64 {
	if y <= 0 {
		return 0
	}
	if y >= 1 {
		return 1
	}
	if y <= 0.04045 {
		return y / 12.92
	}
	return math.Pow((y+0.055)/1.055, 2.4)
}
